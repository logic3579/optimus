package orchestrator

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/delivery/errs"
)

const reconcilePersistenceTimeout = 5 * time.Second

// Inspection is the consumer-owned, closed P3 seam. PreviousDigestProven must
// only be true when P3 can positively prove that the observed revision and
// digest predate the attempted mutation. An arbitrary non-target digest
// (including a later rollback) is not such proof.
type Inspection struct {
	Revision             int64
	Digest               string
	PreviousDigestProven bool
}

type Inspector interface {
	Inspect(context.Context, uint64, string) (Inspection, error)
}

type recoveryIntent uint8

const (
	recoveryFailed recoveryIntent = iota
	recoveryCanceled
	recoveryTimedOut
)

type reconcileCandidate struct {
	Run        models.DeliveryRun
	Stage      models.DeliveryRunStage
	Intent     recoveryIntent
	Generation uint64
}

type reconcileOutcome struct {
	RunState   models.DeliveryRunState
	StageState models.DeliveryStageState
	Revision   int64
	Digest     string
	Drift      bool
	ErrorCode  *int
	ErrorKey   *string
}

type reconcileStore interface {
	Load(context.Context, uint64, time.Time) (*reconcileCandidate, error)
	Resolve(context.Context, reconcileCandidate, reconcileOutcome, time.Time) error
}

type reconcileScanner interface {
	ClaimNextReconcileRun(context.Context, time.Time, time.Duration) (*reconcileClaim, error)
}

type reconcileClaim struct {
	RunID   uint64
	release func(context.Context)
}

func (c *reconcileClaim) Release(ctx context.Context) {
	if c != nil && c.release != nil {
		c.release(ctx)
		c.release = nil
	}
}

type Reconciler struct {
	store     reconcileStore
	inspector Inspector
	now       func() time.Time
}

// Run discovers and reconciles at most one run per interval. Work never
// overlaps, so a slow P3 inspection cannot create an unbounded goroutine set.
func (r *Reconciler) Run(ctx context.Context, interval time.Duration, logger *slog.Logger) {
	if interval <= 0 {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}
	scanner, ok := r.store.(reconcileScanner)
	if !ok {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case tick := <-ticker.C:
			claim, err := scanner.ClaimNextReconcileRun(ctx, tick.UTC(), interval)
			if err == nil && claim != nil {
				err = r.Reconcile(ctx, claim.RunID)
				claim.Release(ctx)
			}
			if err != nil && !errors.Is(err, context.Canceled) {
				logger.Error("delivery reconciliation failed", "category", "database")
			}
		}
	}
}

func NewReconciler(db *gorm.DB, inspector Inspector) *Reconciler {
	if db == nil || inspector == nil {
		panic("orchestrator: invalid reconciler configuration")
	}
	return newReconciler(&gormReconcileStore{db: db}, inspector, time.Now)
}

func newReconciler(store reconcileStore, inspector Inspector, now func() time.Time) *Reconciler {
	return &Reconciler{store: store, inspector: inspector, now: now}
}

// Reconcile resolves one run solely from P3 inspection evidence. Inspection
// errors and ambiguous/non-target observations remain explicitly unknown.
func (r *Reconciler) Reconcile(ctx context.Context, runID uint64) error {
	now := r.now().UTC()
	candidate, err := r.store.Load(ctx, runID, now)
	if err != nil || candidate == nil {
		return err
	}
	evidence, inspectErr := r.inspector.Inspect(ctx, candidate.Stage.ApplicationID, candidate.Stage.OperationID)
	if inspectErr != nil {
		evidence = Inspection{}
	}
	outcome := classifyEvidence(*candidate, evidence, inspectErr)
	persistCtx, cancel := context.WithTimeout(context.Background(), reconcilePersistenceTimeout)
	defer cancel()
	return r.store.Resolve(persistCtx, *candidate, outcome, now)
}

func classifyEvidence(candidate reconcileCandidate, evidence Inspection, inspectErr error) reconcileOutcome {
	if inspectErr == nil && evidence.Revision > 0 && evidence.Digest == candidate.Run.ChartDigest {
		return reconcileOutcome{RunState: models.DeliveryRunSucceeded, StageState: models.DeliveryStageSucceeded, Revision: evidence.Revision, Digest: evidence.Digest}
	}
	previousDigestProven := inspectErr == nil && evidence.PreviousDigestProven &&
		evidence.Revision > 0 && evidence.Digest != "" && evidence.Digest != candidate.Run.ChartDigest
	if previousDigestProven {
		switch candidate.Intent {
		case recoveryCanceled:
			return reconcileOutcome{RunState: models.DeliveryRunCanceled, StageState: models.DeliveryStageCanceled, Revision: evidence.Revision, Digest: evidence.Digest}
		case recoveryTimedOut:
			code, key := recoveryError(errs.CodeExecutionTimeout, errs.KeyExecutionTimeout)
			return reconcileOutcome{RunState: models.DeliveryRunTimedOut, StageState: models.DeliveryStageTimedOut, Revision: evidence.Revision, Digest: evidence.Digest, ErrorCode: code, ErrorKey: key}
		default:
			code, key := recoveryError(errs.CodeExecutionUnavailable, errs.KeyExecutionUnavailable)
			return reconcileOutcome{RunState: models.DeliveryRunFailed, StageState: models.DeliveryStageFailed, Revision: evidence.Revision, Digest: evidence.Digest, ErrorCode: code, ErrorKey: key}
		}
	}
	code, key := recoveryError(errs.CodeOutcomeUnknown, errs.KeyOutcomeUnknown)
	return reconcileOutcome{RunState: models.DeliveryRunOutcomeUnknown, StageState: models.DeliveryStageOutcomeUnknown, Revision: evidence.Revision, Digest: evidence.Digest, Drift: inspectErr == nil && evidence.Digest != "" && evidence.Digest != candidate.Run.ChartDigest, ErrorCode: code, ErrorKey: key}
}

func recoveryError(codeValue apperr.Code, keyValue string) (*int, *string) {
	code := int(codeValue)
	key := keyValue
	return &code, &key
}

type gormReconcileStore struct{ db *gorm.DB }

func (s *gormReconcileStore) ClaimNextReconcileRun(ctx context.Context, now time.Time, retryAfter time.Duration) (*reconcileClaim, error) {
	var runID uint64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		expired := tx.Model(&models.DeliveryRunStage{}).Select("1").
			Where("delivery_run_stages.run_id = delivery_runs.id AND delivery_run_stages.state = ? AND delivery_run_stages.lease_expires_at IS NOT NULL AND delivery_run_stages.lease_expires_at <= ?", models.DeliveryStageRunning, now)
		retryable := tx.Model(&models.DeliveryRunStage{}).Select("1").
			Where("delivery_run_stages.run_id = delivery_runs.id AND delivery_run_stages.state = ? AND delivery_run_stages.updated_at <= ?", models.DeliveryStageReconciling, now.Add(-retryAfter))
		var run models.DeliveryRun
		err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("(state IN ? AND EXISTS (?)) OR (state = ? AND EXISTS (?))", []models.DeliveryRunState{models.DeliveryRunRunning, models.DeliveryRunCancelRequested}, expired, models.DeliveryRunReconciling, retryable).
			Order("CASE WHEN state IN ('running','cancel_requested') THEN 0 ELSE 1 END,id ASC").Limit(1).Take(&run).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}

		var stage models.DeliveryRunStage
		stageQuery := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("run_id = ?", run.ID)
		if run.State == models.DeliveryRunRunning || run.State == models.DeliveryRunCancelRequested {
			stageQuery = stageQuery.Where("state = ? AND lease_expires_at IS NOT NULL AND lease_expires_at <= ?", models.DeliveryStageRunning, now)
		} else {
			stageQuery = stageQuery.Where("state = ? AND updated_at <= ?", models.DeliveryStageReconciling, now.Add(-retryAfter))
		}
		if err := stageQuery.Order("stage_order ASC,id ASC").Take(&stage).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		} else if err != nil {
			return err
		}
		if run.State == models.DeliveryRunReconciling {
			res := tx.Model(&models.DeliveryRunStage{}).Where("id = ? AND state = ? AND updated_at <= ?", stage.ID, models.DeliveryStageReconciling, now.Add(-retryAfter)).Update("updated_at", now)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected != 1 {
				return nil
			}
			runID = run.ID
			return nil
		}

		intent := "lease_lost"
		if run.State == models.DeliveryRunCancelRequested {
			intent = "canceled"
		}
		metadata, _ := json.Marshal(map[string]any{"recovery_intent": intent})
		oldRunState := run.State
		if err := transitionStage(tx, &stage, models.DeliveryStageReconciling, now, map[string]any{"lease_owner": nil, "lease_expires_at": nil}); err != nil {
			return err
		}
		if err := transitionRun(tx, &run, models.DeliveryRunReconciling, now, nil); err != nil {
			return err
		}
		if err := appendTransition(tx, run.ID, nil, "run.reconciling", oldRunState, models.DeliveryRunReconciling, now, nil, nil, datatypes.JSON(metadata)); err != nil {
			return err
		}
		if err := appendTransition(tx, run.ID, &stage.ID, "stage.reconciling", models.DeliveryStageRunning, models.DeliveryStageReconciling, now, nil, nil, datatypes.JSON(metadata)); err != nil {
			return err
		}
		runID = run.ID
		return nil
	})
	if err != nil || runID == 0 {
		return nil, err
	}
	return s.advisoryClaim(ctx, runID)
}

const reconcileAdvisoryNamespace int64 = 0x5046523600000000
const reconcileUnlockTimeout = time.Second

func reconcileAdvisoryKey(runID uint64) (int64, error) {
	if runID > math.MaxInt64 {
		return 0, fmt.Errorf("delivery run ID %d exceeds advisory lock range", runID)
	}
	return reconcileAdvisoryNamespace ^ int64(runID), nil // #nosec G115 -- range checked above
}

func (s *gormReconcileStore) advisoryClaim(ctx context.Context, runID uint64) (*reconcileClaim, error) {
	sqlDB, err := s.db.DB()
	if err != nil {
		return nil, err
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return nil, err
	}
	key, err := reconcileAdvisoryKey(runID)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	var acquired bool
	if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&acquired); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if !acquired {
		_ = conn.Close()
		return nil, nil
	}
	return &reconcileClaim{RunID: runID, release: func(parent context.Context) {
		boundedAdvisoryRelease(parent, reconcileUnlockTimeout, func(unlockCtx context.Context) (bool, error) {
			var unlocked bool
			err := conn.QueryRowContext(unlockCtx, "SELECT pg_advisory_unlock($1)", key).Scan(&unlocked)
			return unlocked, err
		}, func() { _ = conn.Raw(func(any) error { return driver.ErrBadConn }) })
		_ = conn.Close()
	}}, nil
}

func boundedAdvisoryRelease(parent context.Context, timeout time.Duration, unlock func(context.Context) (bool, error), discard func()) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	unlocked, err := unlock(ctx)
	if err != nil || !unlocked {
		discard()
	}
}

func (s *gormReconcileStore) Load(ctx context.Context, runID uint64, _ time.Time) (*reconcileCandidate, error) {
	var run models.DeliveryRun
	if err := s.db.WithContext(ctx).First(&run, runID).Error; err != nil {
		return nil, err
	}
	var stage models.DeliveryRunStage
	err := s.db.WithContext(ctx).Where("run_id = ? AND state = ?", runID, models.DeliveryStageReconciling).Order("stage_order ASC,id ASC").First(&stage).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var transitions []models.DeliveryRunEvent
	if err := s.db.WithContext(ctx).
		Where("run_id = ? AND run_stage_id = ? AND event_type = ?", runID, stage.ID, "stage.reconciling").
		Order("id DESC").Find(&transitions).Error; err != nil {
		return nil, err
	}
	if len(transitions) == 0 {
		return nil, nil
	}
	intent := recoveryIntentFromEvents(transitions)
	var canceled int64
	if err := s.db.WithContext(ctx).Model(&models.DeliveryRunEvent{}).Where("run_id = ? AND event_type = ?", runID, "run.cancel_requested").Count(&canceled).Error; err != nil {
		return nil, err
	}
	if intent == recoveryFailed && canceled > 0 {
		intent = recoveryCanceled
	}
	return &reconcileCandidate{Run: run, Stage: stage, Intent: intent, Generation: transitions[0].ID}, nil
}

func recoveryIntentFromEvents(events []models.DeliveryRunEvent) recoveryIntent {
	for i := range events {
		var metadata struct {
			RecoveryIntent string `json:"recovery_intent"`
		}
		if json.Unmarshal(events[i].Metadata, &metadata) != nil {
			continue
		}
		switch metadata.RecoveryIntent {
		case "canceled":
			return recoveryCanceled
		case "timed_out":
			return recoveryTimedOut
		case "failed":
			return recoveryFailed
		case "lease_lost":
			return recoveryFailed
		}
	}
	return recoveryFailed
}

func (s *gormReconcileStore) Resolve(ctx context.Context, expected reconcileCandidate, outcome reconcileOutcome, now time.Time) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run models.DeliveryRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, expected.Run.ID).Error; err != nil {
			return err
		}
		var stage models.DeliveryRunStage
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&stage, expected.Stage.ID).Error; err != nil {
			return err
		}
		if run.State != models.DeliveryRunReconciling || stage.State != models.DeliveryStageReconciling {
			return nil
		}
		var generation uint64
		if err := tx.Model(&models.DeliveryRunEvent{}).
			Select("id").Where("run_id = ? AND run_stage_id = ? AND event_type = ?", run.ID, stage.ID, "stage.reconciling").
			Order("id DESC").Limit(1).Scan(&generation).Error; err != nil {
			return err
		}
		if !sameReconcileGeneration(expected.Generation, generation) {
			return nil
		}
		stageExtra := map[string]any{"finished_at": now, "lease_owner": nil, "lease_expires_at": nil}
		if outcome.ErrorCode != nil {
			stageExtra["error_code"] = *outcome.ErrorCode
			stageExtra["error_message_key"] = *outcome.ErrorKey
		}
		if outcome.Revision > 0 {
			stageExtra["result_revision"] = outcome.Revision
		}
		if outcome.Digest != "" {
			stageExtra["result_digest"] = outcome.Digest
		}
		if err := transitionStage(tx, &stage, outcome.StageState, now, stageExtra); err != nil {
			return err
		}
		metadata, _ := json.Marshal(map[string]any{"release_revision": outcome.Revision, "observed_digest": outcome.Digest, "drift": outcome.Drift})
		if err := appendTransition(tx, run.ID, &stage.ID, "stage."+string(outcome.StageState), models.DeliveryStageReconciling, outcome.StageState, now, outcome.ErrorCode, outcome.ErrorKey, datatypes.JSON(metadata)); err != nil {
			return err
		}
		if outcome.StageState == models.DeliveryStageSucceeded {
			return s.advanceAfterSuccess(tx, &run, &stage, now)
		}
		old := run.State
		runExtra := map[string]any{"finished_at": now}
		if outcome.ErrorCode != nil {
			runExtra["error_code"] = *outcome.ErrorCode
			runExtra["error_message_key"] = *outcome.ErrorKey
		}
		if err := transitionRun(tx, &run, outcome.RunState, now, runExtra); err != nil {
			return err
		}
		return appendTransition(tx, run.ID, nil, "run."+string(outcome.RunState), old, outcome.RunState, now, outcome.ErrorCode, outcome.ErrorKey, datatypes.JSON(metadata))
	})
}

func sameReconcileGeneration(expected, current uint64) bool {
	return expected != 0 && expected == current
}

func (s *gormReconcileStore) advanceAfterSuccess(tx *gorm.DB, run *models.DeliveryRun, stage *models.DeliveryRunStage, now time.Time) error {
	var next models.DeliveryRunStage
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("run_id = ? AND stage_order > ?", run.ID, stage.StageOrder).Order("stage_order ASC,id ASC").First(&next).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		old := run.State
		if err := transitionRun(tx, run, models.DeliveryRunSucceeded, now, map[string]any{"finished_at": now}); err != nil {
			return err
		}
		return appendTransition(tx, run.ID, nil, "run.succeeded", old, models.DeliveryRunSucceeded, now, nil, nil)
	}
	if err != nil {
		return err
	}
	nextState, runState := models.DeliveryStageQueued, models.DeliveryRunRunning
	if next.ApprovalRequired {
		nextState, runState = models.DeliveryStageWaitingApproval, models.DeliveryRunWaitingApproval
		if err := tx.Create(&models.DeliveryApproval{RunID: run.ID, RunStageID: next.ID, RequestedAt: now, Decision: models.DeliveryApprovalPending, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
			return err
		}
	}
	if err := transitionStage(tx, &next, nextState, now, nil); err != nil {
		return err
	}
	oldRun := run.State
	if err := transitionRun(tx, run, runState, now, nil); err != nil {
		return err
	}
	if err := appendTransition(tx, run.ID, &next.ID, "stage."+string(nextState), models.DeliveryStagePending, nextState, now, nil, nil); err != nil {
		return err
	}
	return appendTransition(tx, run.ID, nil, "run."+string(runState), oldRun, runState, now, nil, nil)
}
