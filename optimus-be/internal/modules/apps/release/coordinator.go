package release

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
)

const (
	operationBusyKey          = "delivery.execution.operation_busy"
	reconciliationRequiredKey = "delivery.execution.reconciliation_required"
	executionUnavailableKey   = "delivery.execution.unavailable"
)

var (
	errOperationNotFound = errors.New("release operation not found")
	errOperationConflict = errors.New("release operation conflict")
	operationDigestRE    = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

// Operation is the durable, safe release-operation record. It contains only
// identifiers, lease metadata, and a definite revision/digest result.
type Operation = models.AppsReleaseOperation

// SafeOperationResult is the only completion payload persisted by the
// coordinator. Callers must classify upstream failures before completing an
// operation; raw error text, values, chart bytes, and executor output have no
// field in this type or in the backing table.
type SafeOperationResult struct {
	Succeeded bool
	Revision  int64
	Digest    string
}

// AcquireResult distinguishes permission to mutate from an idempotent replay
// and from an expired lease that must be reconciled before any new mutation.
type AcquireResult struct {
	Operation           *Operation
	Acquired            bool
	Replayed            bool
	NeedsReconciliation bool
}

// Coordinator serializes P3 release mutations through durable database
// leases. A Coordinator is safe to use across goroutines; separate instances
// coordinate through PostgreSQL row locks.
type Coordinator struct {
	store operationStore
	now   func() time.Time
}

// NewCoordinator constructs a database-backed release operation coordinator.
func NewCoordinator(db *gorm.DB) *Coordinator {
	if db == nil {
		panic("release: NewCoordinator: db is nil")
	}
	return newCoordinator(&gormOperationStore{db: db}, time.Now)
}

func newCoordinator(store operationStore, now func() time.Time) *Coordinator {
	if store == nil {
		panic("release: newCoordinator: store is nil")
	}
	if now == nil {
		panic("release: newCoordinator: clock is nil")
	}
	return &Coordinator{store: store, now: now}
}

// Acquire creates or replays a stable operation. Acquired is the sole
// authorization to start or continue an upstream mutation. Expired active
// rows move to reconciling and never grant a takeover.
func (c *Coordinator) Acquire(
	ctx context.Context,
	applicationID uint64,
	operationID, kind, owner string,
	lease time.Duration,
) (AcquireResult, error) {
	if applicationID == 0 || !validBounded(operationID, 64) || !validBounded(kind, 64) ||
		!validBounded(owner, 128) || lease <= 0 {
		return AcquireResult{}, executionUnavailableError()
	}

	now := c.now().UTC()
	var result AcquireResult
	err := c.store.WithApplicationLock(ctx, applicationID, func(tx operationTransaction) error {
		existing, err := tx.FindByOperationID(ctx, operationID)
		switch {
		case err == nil:
			if existing.ApplicationID != applicationID || existing.Kind != kind {
				return operationBusyError()
			}
			result = AcquireResult{Operation: existing, Replayed: true}
			switch existing.State {
			case models.AppsReleaseOperationSucceeded, models.AppsReleaseOperationFailed:
				return nil
			case models.AppsReleaseOperationReconciling:
				if existing.LeaseOwner == nil || *existing.LeaseOwner != owner {
					return operationBusyError()
				}
				result.NeedsReconciliation = true
				return nil
			case models.AppsReleaseOperationActive:
				if leaseExpired(existing, now) {
					if err := markReconciling(ctx, tx, existing, owner, now); err != nil {
						return err
					}
					result.Operation = existing
					result.NeedsReconciliation = true
					return nil
				}
				if existing.LeaseOwner != nil && *existing.LeaseOwner == owner {
					until := now.Add(lease)
					if existing.LeaseExpiresAt == nil || existing.LeaseExpiresAt.Before(until) {
						existing.LeaseExpiresAt = &until
						existing.UpdatedAt = now
						if err := tx.Save(ctx, existing); err != nil {
							return err
						}
					}
					result.Acquired = true
				}
				return nil
			default:
				return executionUnavailableError()
			}
		case !errors.Is(err, errOperationNotFound):
			return err
		}

		blocking, err := tx.FindBlockingByApplication(ctx, applicationID)
		switch {
		case err == nil:
			if blocking.State == models.AppsReleaseOperationReconciling {
				if blocking.LeaseOwner == nil || *blocking.LeaseOwner != owner {
					return operationBusyError()
				}
				result = AcquireResult{Operation: blocking, NeedsReconciliation: true}
				return nil
			}
			if leaseExpired(blocking, now) {
				if err := markReconciling(ctx, tx, blocking, owner, now); err != nil {
					return err
				}
				result = AcquireResult{Operation: blocking, NeedsReconciliation: true}
				return nil
			}
			return operationBusyError()
		case !errors.Is(err, errOperationNotFound):
			return err
		}

		until := now.Add(lease)
		ownerCopy := owner
		row := &models.AppsReleaseOperation{
			ApplicationID:  applicationID,
			OperationID:    operationID,
			Kind:           kind,
			State:          models.AppsReleaseOperationActive,
			LeaseOwner:     &ownerCopy,
			LeaseExpiresAt: &until,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := tx.Create(ctx, row); err != nil {
			if errors.Is(err, errOperationConflict) {
				return operationBusyError()
			}
			return err
		}
		result = AcquireResult{Operation: row, Acquired: true}
		return nil
	})
	if err != nil {
		return AcquireResult{}, mapCoordinatorStoreError(err)
	}
	return result, nil
}

// Renew extends an unexpired active lease. Only its current owner may renew.
func (c *Coordinator) Renew(ctx context.Context, operationID, owner string, until time.Time) error {
	if !validBounded(operationID, 64) || !validBounded(owner, 128) || until.IsZero() {
		return executionUnavailableError()
	}
	now := c.now().UTC()
	until = until.UTC()
	if !until.After(now) {
		return executionUnavailableError()
	}
	var committedError error
	err := c.withLockedOperation(ctx, operationID, func(tx operationTransaction, row *models.AppsReleaseOperation) error {
		if row.State != models.AppsReleaseOperationActive {
			if row.State == models.AppsReleaseOperationReconciling {
				if row.LeaseOwner == nil || *row.LeaseOwner != owner {
					return operationBusyError()
				}
				return reconciliationRequiredError()
			}
			return operationBusyError()
		}
		if leaseExpired(row, now) {
			if err := markReconciling(ctx, tx, row, owner, now); err != nil {
				return err
			}
			committedError = reconciliationRequiredError()
			return nil
		}
		if row.LeaseOwner == nil || *row.LeaseOwner != owner {
			return operationBusyError()
		}
		if row.LeaseExpiresAt == nil || row.LeaseExpiresAt.Before(until) {
			row.LeaseExpiresAt = &until
			row.UpdatedAt = now
			return tx.Save(ctx, row)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return committedError
}

// Complete records a definite terminal result. Completion is denied after
// lease loss, because the upstream outcome must first be reconciled.
func (c *Coordinator) Complete(ctx context.Context, operationID, owner string, result SafeOperationResult) error {
	if !validBounded(operationID, 64) || !validBounded(owner, 128) || !validSafeOperationResult(result) {
		return executionUnavailableError()
	}
	now := c.now().UTC()
	var committedError error
	err := c.withLockedOperation(ctx, operationID, func(tx operationTransaction, row *models.AppsReleaseOperation) error {
		switch row.State {
		case models.AppsReleaseOperationReconciling:
			if row.LeaseOwner == nil || *row.LeaseOwner != owner {
				return operationBusyError()
			}
		case models.AppsReleaseOperationActive:
			if leaseExpired(row, now) {
				if err := markReconciling(ctx, tx, row, owner, now); err != nil {
					return err
				}
				committedError = reconciliationRequiredError()
				return nil
			}
			if row.LeaseOwner == nil || *row.LeaseOwner != owner {
				return operationBusyError()
			}
		default:
			return operationBusyError()
		}

		row.State = models.AppsReleaseOperationFailed
		if result.Succeeded {
			row.State = models.AppsReleaseOperationSucceeded
		}
		if result.Revision != 0 {
			revision := result.Revision
			row.ResultRevision = &revision
		}
		if result.Digest != "" {
			digest := result.Digest
			row.ResultDigest = &digest
		}
		row.FinishedAt = &now
		row.UpdatedAt = now
		return tx.Save(ctx, row)
	})
	if err != nil {
		return err
	}
	return committedError
}

// Inspect returns a safe durable operation snapshot by stable operation ID.
func (c *Coordinator) Inspect(ctx context.Context, operationID string) (*Operation, error) {
	if !validBounded(operationID, 64) {
		return nil, executionUnavailableError()
	}
	row, err := c.store.Inspect(ctx, operationID)
	if err != nil {
		return nil, mapCoordinatorStoreError(err)
	}
	return row, nil
}

func (c *Coordinator) withLockedOperation(
	ctx context.Context,
	operationID string,
	fn func(operationTransaction, *models.AppsReleaseOperation) error,
) error {
	snapshot, err := c.store.Inspect(ctx, operationID)
	if err != nil {
		return mapCoordinatorStoreError(err)
	}
	err = c.store.WithApplicationLock(ctx, snapshot.ApplicationID, func(tx operationTransaction) error {
		row, err := tx.FindByOperationID(ctx, operationID)
		if err != nil {
			return err
		}
		return fn(tx, row)
	})
	return mapCoordinatorStoreError(err)
}

func leaseExpired(row *models.AppsReleaseOperation, now time.Time) bool {
	return row.LeaseExpiresAt == nil || !row.LeaseExpiresAt.After(now)
}

func markReconciling(
	ctx context.Context,
	tx operationTransaction,
	row *models.AppsReleaseOperation,
	owner string,
	now time.Time,
) error {
	ownerCopy := owner
	row.State = models.AppsReleaseOperationReconciling
	row.LeaseOwner = &ownerCopy
	row.LeaseExpiresAt = nil
	row.UpdatedAt = now
	return tx.Save(ctx, row)
}

func validSafeOperationResult(result SafeOperationResult) bool {
	if result.Succeeded {
		return result.Revision > 0 && operationDigestRE.MatchString(result.Digest)
	}
	return result.Revision == 0 && result.Digest == ""
}

func validBounded(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && strings.TrimSpace(value) == value
}

func operationBusyError() error {
	return apperr.New(apperr.CodeDeliveryOperationBusy, operationBusyKey, "release operation is busy")
}

func reconciliationRequiredError() error {
	return apperr.New(apperr.CodeDeliveryReconciliationRequired, reconciliationRequiredKey, "release operation requires reconciliation")
}

func executionUnavailableError() error {
	return apperr.New(apperr.CodeDeliveryExecutionUnavailable, executionUnavailableKey, "release operation is unavailable")
}

func mapCoordinatorStoreError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, errOperationNotFound):
		return executionUnavailableError()
	case errors.Is(err, errOperationConflict):
		return operationBusyError()
	default:
		if _, ok := apperr.AsBiz(err); ok {
			return err
		}
		return executionUnavailableError()
	}
}

type operationStore interface {
	WithApplicationLock(context.Context, uint64, func(operationTransaction) error) error
	Inspect(context.Context, string) (*models.AppsReleaseOperation, error)
}

type operationTransaction interface {
	FindByOperationID(context.Context, string) (*models.AppsReleaseOperation, error)
	FindBlockingByApplication(context.Context, uint64) (*models.AppsReleaseOperation, error)
	Create(context.Context, *models.AppsReleaseOperation) error
	Save(context.Context, *models.AppsReleaseOperation) error
}

type gormOperationStore struct{ db *gorm.DB }

func (s *gormOperationStore) WithApplicationLock(
	ctx context.Context,
	applicationID uint64,
	fn func(operationTransaction) error,
) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var application models.AppsApplication
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").First(&application, applicationID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errOperationNotFound
			}
			return err
		}
		return fn(&gormOperationTransaction{db: tx})
	})
}

func (s *gormOperationStore) Inspect(ctx context.Context, operationID string) (*models.AppsReleaseOperation, error) {
	var row models.AppsReleaseOperation
	if err := s.db.WithContext(ctx).Where("operation_id = ?", operationID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errOperationNotFound
		}
		return nil, err
	}
	return &row, nil
}

type gormOperationTransaction struct{ db *gorm.DB }

func (tx *gormOperationTransaction) FindByOperationID(ctx context.Context, operationID string) (*models.AppsReleaseOperation, error) {
	var row models.AppsReleaseOperation
	err := tx.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("operation_id = ?", operationID).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errOperationNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (tx *gormOperationTransaction) FindBlockingByApplication(ctx context.Context, applicationID uint64) (*models.AppsReleaseOperation, error) {
	var row models.AppsReleaseOperation
	err := tx.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("application_id = ? AND state IN ?", applicationID, []models.AppsReleaseOperationState{
			models.AppsReleaseOperationActive,
			models.AppsReleaseOperationReconciling,
		}).
		Order("id DESC").
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errOperationNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (tx *gormOperationTransaction) Create(ctx context.Context, row *models.AppsReleaseOperation) error {
	err := tx.db.WithContext(ctx).Create(row).Error
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		switch pgErr.ConstraintName {
		case "apps_release_operations_operation_unique", "apps_release_operations_active_application_unique":
			return errOperationConflict
		}
	}
	return err
}

func (tx *gormOperationTransaction) Save(ctx context.Context, row *models.AppsReleaseOperation) error {
	return tx.db.WithContext(ctx).Save(row).Error
}
