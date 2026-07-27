package release

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
)

func TestCoordinatorAcquireFirstOperation(t *testing.T) {
	coordinator, _, now := newCoordinatorHarness()

	result, err := coordinator.Acquire(context.Background(), 41, "operation-1", "upgrade", "worker-1", time.Minute)
	require.NoError(t, err)
	require.True(t, result.Acquired)
	require.False(t, result.Replayed)
	require.False(t, result.NeedsReconciliation)
	require.Equal(t, "operation-1", result.Operation.OperationID)
	require.Equal(t, models.AppsReleaseOperationActive, result.Operation.State)
	require.Equal(t, "worker-1", derefString(result.Operation.LeaseOwner))
	require.Equal(t, now.Add(time.Minute), derefTime(result.Operation.LeaseExpiresAt))
}

func TestCoordinatorAcquireSameOperationReplays(t *testing.T) {
	coordinator, _, _ := newCoordinatorHarness()
	ctx := context.Background()

	first, err := coordinator.Acquire(ctx, 41, "operation-1", "upgrade", "worker-1", time.Minute)
	require.NoError(t, err)
	replay, err := coordinator.Acquire(ctx, 41, "operation-1", "upgrade", "worker-1", time.Minute)
	require.NoError(t, err)
	require.True(t, replay.Acquired)
	require.True(t, replay.Replayed)
	require.Equal(t, first.Operation.ID, replay.Operation.ID)

	otherOwner, err := coordinator.Acquire(ctx, 41, "operation-1", "upgrade", "worker-2", time.Minute)
	require.NoError(t, err)
	require.False(t, otherOwner.Acquired)
	require.True(t, otherOwner.Replayed)
	require.Equal(t, "worker-1", derefString(otherOwner.Operation.LeaseOwner))
}

func TestCoordinatorAcquireDifferentOperationReturnsBusy(t *testing.T) {
	coordinator, _, _ := newCoordinatorHarness()
	ctx := context.Background()

	_, err := coordinator.Acquire(ctx, 41, "operation-1", "upgrade", "worker-1", time.Minute)
	require.NoError(t, err)
	_, err = coordinator.Acquire(ctx, 41, "operation-2", "rollback", "worker-2", time.Minute)
	requireBizError(t, err, apperr.CodeDeliveryOperationBusy, "delivery.execution.operation_busy")
}

func TestCoordinatorRenewRequiresLeaseOwner(t *testing.T) {
	coordinator, _, now := newCoordinatorHarness()
	ctx := context.Background()

	_, err := coordinator.Acquire(ctx, 41, "operation-1", "upgrade", "worker-1", time.Minute)
	require.NoError(t, err)
	requireBizError(t,
		coordinator.Renew(ctx, "operation-1", "worker-2", now.Add(2*time.Minute)),
		apperr.CodeDeliveryOperationBusy,
		"delivery.execution.operation_busy",
	)
	require.NoError(t, coordinator.Renew(ctx, "operation-1", "worker-1", now.Add(2*time.Minute)))

	operation, err := coordinator.Inspect(ctx, "operation-1")
	require.NoError(t, err)
	require.Equal(t, now.Add(2*time.Minute), derefTime(operation.LeaseExpiresAt))
}

func TestCoordinatorLostOwnerCannotComplete(t *testing.T) {
	coordinator, clock, _ := newCoordinatorHarness()
	ctx := context.Background()

	_, err := coordinator.Acquire(ctx, 41, "operation-1", "upgrade", "worker-1", time.Minute)
	require.NoError(t, err)
	clock.Advance(2 * time.Minute)

	err = coordinator.Complete(ctx, "operation-1", "worker-1", SafeOperationResult{
		Succeeded: true,
		Revision:  7,
		Digest:    "sha256:safe",
	})
	requireBizError(t, err, apperr.CodeDeliveryReconciliationRequired, "delivery.execution.reconciliation_required")
	operation, err := coordinator.Inspect(ctx, "operation-1")
	require.NoError(t, err)
	require.Equal(t, models.AppsReleaseOperationReconciling, operation.State)
	require.Nil(t, operation.ResultRevision)
	require.Nil(t, operation.ResultDigest)
}

func TestCoordinatorCompleteStoresOnlySafeResult(t *testing.T) {
	coordinator, _, _ := newCoordinatorHarness()
	ctx := context.Background()

	_, err := coordinator.Acquire(ctx, 41, "operation-1", "upgrade", "worker-1", time.Minute)
	require.NoError(t, err)
	require.NoError(t, coordinator.Complete(ctx, "operation-1", "worker-1", SafeOperationResult{
		Succeeded: true,
		Revision:  7,
		Digest:    "sha256:safe",
	}))

	operation, err := coordinator.Inspect(ctx, "operation-1")
	require.NoError(t, err)
	require.Equal(t, models.AppsReleaseOperationSucceeded, operation.State)
	require.EqualValues(t, 7, derefInt64(operation.ResultRevision))
	require.Equal(t, "sha256:safe", derefString(operation.ResultDigest))
	require.NotNil(t, operation.FinishedAt)

	replay, err := coordinator.Acquire(ctx, 41, "operation-1", "upgrade", "worker-2", time.Minute)
	require.NoError(t, err)
	require.False(t, replay.Acquired)
	require.True(t, replay.Replayed)
	require.Equal(t, models.AppsReleaseOperationSucceeded, replay.Operation.State)
}

func TestCoordinatorExpiredLeaseRequiresReconciliationWithoutTakeover(t *testing.T) {
	coordinator, clock, _ := newCoordinatorHarness()
	ctx := context.Background()

	_, err := coordinator.Acquire(ctx, 41, "operation-1", "upgrade", "worker-1", time.Minute)
	require.NoError(t, err)
	clock.Advance(2 * time.Minute)

	result, err := coordinator.Acquire(ctx, 41, "operation-2", "upgrade", "worker-2", time.Minute)
	require.NoError(t, err)
	require.False(t, result.Acquired)
	require.False(t, result.Replayed)
	require.True(t, result.NeedsReconciliation)
	require.Equal(t, "operation-1", result.Operation.OperationID)
	require.Equal(t, models.AppsReleaseOperationReconciling, result.Operation.State)
	require.Equal(t, "worker-1", derefString(result.Operation.LeaseOwner))

	_, err = coordinator.Acquire(ctx, 41, "operation-2", "upgrade", "worker-2", time.Minute)
	requireBizError(t, err, apperr.CodeDeliveryReconciliationRequired, "delivery.execution.reconciliation_required")
}

func newCoordinatorHarness() (*Coordinator, *fakeClock, time.Time) {
	now := time.Date(2026, time.July, 27, 12, 0, 0, 0, time.UTC)
	clock := &fakeClock{now: now}
	return newCoordinator(newMemoryOperationStore(), clock.Now), clock, now
}

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type memoryOperationStore struct {
	mu            sync.Mutex
	nextID        uint64
	byOperation   map[string]*models.AppsReleaseOperation
	byApplication map[uint64][]*models.AppsReleaseOperation
}

func newMemoryOperationStore() *memoryOperationStore {
	return &memoryOperationStore{
		nextID:        1,
		byOperation:   make(map[string]*models.AppsReleaseOperation),
		byApplication: make(map[uint64][]*models.AppsReleaseOperation),
	}
}

func (s *memoryOperationStore) WithApplicationLock(_ context.Context, _ uint64, fn func(operationTransaction) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fn((*memoryOperationTransaction)(s))
}

func (s *memoryOperationStore) Inspect(_ context.Context, operationID string) (*models.AppsReleaseOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row := s.byOperation[operationID]
	if row == nil {
		return nil, errOperationNotFound
	}
	return cloneOperation(row), nil
}

type memoryOperationTransaction memoryOperationStore

func (tx *memoryOperationTransaction) FindByOperationID(_ context.Context, operationID string) (*models.AppsReleaseOperation, error) {
	row := tx.byOperation[operationID]
	if row == nil {
		return nil, errOperationNotFound
	}
	return cloneOperation(row), nil
}

func (tx *memoryOperationTransaction) FindBlockingByApplication(_ context.Context, applicationID uint64) (*models.AppsReleaseOperation, error) {
	for _, row := range tx.byApplication[applicationID] {
		if row.State == models.AppsReleaseOperationActive || row.State == models.AppsReleaseOperationReconciling {
			return cloneOperation(row), nil
		}
	}
	return nil, errOperationNotFound
}

func (tx *memoryOperationTransaction) Create(_ context.Context, row *models.AppsReleaseOperation) error {
	if _, exists := tx.byOperation[row.OperationID]; exists {
		return errOperationConflict
	}
	row.ID = tx.nextID
	tx.nextID++
	stored := cloneOperation(row)
	tx.byOperation[row.OperationID] = stored
	tx.byApplication[row.ApplicationID] = append(tx.byApplication[row.ApplicationID], stored)
	return nil
}

func (tx *memoryOperationTransaction) Save(_ context.Context, row *models.AppsReleaseOperation) error {
	stored := tx.byOperation[row.OperationID]
	if stored == nil {
		return errOperationNotFound
	}
	*stored = *cloneOperation(row)
	return nil
}

func cloneOperation(row *models.AppsReleaseOperation) *models.AppsReleaseOperation {
	if row == nil {
		return nil
	}
	clone := *row
	if row.LeaseOwner != nil {
		value := *row.LeaseOwner
		clone.LeaseOwner = &value
	}
	if row.LeaseExpiresAt != nil {
		value := *row.LeaseExpiresAt
		clone.LeaseExpiresAt = &value
	}
	if row.ResultRevision != nil {
		value := *row.ResultRevision
		clone.ResultRevision = &value
	}
	if row.ResultDigest != nil {
		value := *row.ResultDigest
		clone.ResultDigest = &value
	}
	if row.FinishedAt != nil {
		value := *row.FinishedAt
		clone.FinishedAt = &value
	}
	return &clone
}

func requireBizError(t *testing.T, err error, code apperr.Code, key string) {
	t.Helper()
	require.Error(t, err)
	got, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, code, got.Code)
	require.Equal(t, key, got.MessageKey)
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func derefTime(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

func derefInt64(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
