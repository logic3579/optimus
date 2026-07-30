package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/delivery/errs"
	deliveryrun "optimus-be/internal/modules/delivery/run"
)

type Config struct {
	Concurrency, BatchSize                     int
	LeaseDuration, RenewInterval, PollInterval time.Duration
	PersistenceTimeout                         time.Duration
}

type clock interface {
	Now() time.Time
	NewTicker(time.Duration) ticker
}
type ticker interface {
	Chan() <-chan time.Time
	Stop()
}
type realClock struct{}

func (realClock) Now() time.Time                   { return time.Now().UTC() }
func (realClock) NewTicker(d time.Duration) ticker { return realTicker{Ticker: time.NewTicker(d)} }

type realTicker struct{ *time.Ticker }

func (t realTicker) Chan() <-chan time.Time { return t.C }

type claimedWork struct {
	Run   models.DeliveryRun
	Stage models.DeliveryRunStage
}
type completion struct {
	result    UpgradeResult
	err       error
	ambiguous bool
	intent    recoveryIntent
}

type store interface {
	Claim(context.Context, string, time.Time, time.Duration) (*claimedWork, error)
	Renew(context.Context, uint64, string, time.Time) (bool, bool, error)
	Complete(context.Context, claimedWork, string, time.Time, completion) error
}

type Worker struct {
	store    store
	executor Executor
	cfg      Config
	owner    string
	clock    clock
	sem      chan struct{}
	wg       sync.WaitGroup
}

func NewWorker(db *gorm.DB, executor Executor, cfg Config, owner string) *Worker {
	if db == nil || executor == nil || owner == "" || cfg.Concurrency <= 0 || cfg.LeaseDuration <= 0 || cfg.RenewInterval <= 0 {
		panic("orchestrator: invalid worker configuration")
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 250 * time.Millisecond
	}
	if cfg.PersistenceTimeout <= 0 {
		cfg.PersistenceTimeout = 5 * time.Second
	}
	return newWorker(&gormStore{db: db}, executor, cfg, owner, realClock{})
}

func newWorker(s store, executor Executor, cfg Config, owner string, c clock) *Worker {
	return &Worker{store: s, executor: executor, cfg: cfg, owner: owner, clock: c, sem: make(chan struct{}, cfg.Concurrency)}
}

// Run stops claiming immediately on cancellation and waits for local work to leave safely.
func (w *Worker) Run(ctx context.Context) {
	ticker := w.clock.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			w.wg.Wait()
			return
		default:
		}
		select {
		case w.sem <- struct{}{}:
			work, err := w.store.Claim(ctx, w.owner, w.clock.Now(), w.cfg.LeaseDuration)
			if err != nil || work == nil {
				<-w.sem
			} else {
				w.wg.Add(1)
				go w.execute(ctx, *work)
			}
		default:
		}
		select {
		case <-ctx.Done():
			w.wg.Wait()
			return
		case <-ticker.Chan():
		}
	}
}

// ProcessOnce claims and synchronously processes at most one stage.
func (w *Worker) ProcessOnce(ctx context.Context) error {
	work, err := w.store.Claim(ctx, w.owner, w.clock.Now(), w.cfg.LeaseDuration)
	if err != nil || work == nil {
		return err
	}
	_, err = w.executeSync(ctx, *work)
	return err
}

func (w *Worker) execute(parent context.Context, work claimedWork) {
	defer w.wg.Done()
	detached, _ := w.executeSync(parent, work)
	if detached == nil {
		<-w.sem
		return
	}
	// The worker may shut down without trusting an unresponsive executor, but
	// its concurrency token remains occupied until that execution really ends.
	go func() {
		<-detached
		<-w.sem
	}()
}

func (w *Worker) executeSync(parent context.Context, work claimedWork) (<-chan struct{}, error) {
	timeout := time.Duration(work.Stage.TimeoutSeconds) * time.Second
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	resultCh := make(chan completion, 1)
	executorDone := make(chan struct{})
	go func() {
		defer close(executorDone)
		result, err := w.executor.UpgradeExisting(ctx, requestFor(work, timeout))
		resultCh <- completion{result: result, err: err, ambiguous: isAmbiguous(err)}
	}()
	ticker := w.clock.NewTicker(w.cfg.RenewInterval)
	defer ticker.Stop()
	for {
		select {
		case outcome := <-resultCh:
			if ctx.Err() != nil {
				outcome.ambiguous = true
				if errors.Is(ctx.Err(), context.DeadlineExceeded) {
					outcome.intent = recoveryTimedOut
				}
			}
			return nil, w.complete(work, outcome)
		case <-ticker.Chan():
			persistCtx, persistCancel := context.WithTimeout(context.Background(), w.cfg.PersistenceTimeout)
			ok, cancelRequested, err := w.store.Renew(persistCtx, work.Stage.ID, w.owner, w.clock.Now().Add(w.cfg.LeaseDuration))
			persistCancel()
			if cancelRequested {
				cancel()
				return executorDone, w.complete(work, completion{err: context.Canceled, ambiguous: true, intent: recoveryCanceled})
			}
			if err != nil || !ok {
				cancel()
				return executorDone, w.complete(work, completion{err: err, ambiguous: true})
			}
		case <-ctx.Done():
			intent := recoveryFailed
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				intent = recoveryTimedOut
			}
			return executorDone, w.complete(work, completion{err: ctx.Err(), ambiguous: true, intent: intent})
		}
	}
}

func (w *Worker) complete(work claimedWork, outcome completion) error {
	ctx, cancel := context.WithTimeout(context.Background(), w.cfg.PersistenceTimeout)
	defer cancel()
	return w.store.Complete(ctx, work, w.owner, w.clock.Now(), outcome)
}

func requestFor(w claimedWork, timeout time.Duration) UpgradeRequest {
	return UpgradeRequest{ApplicationID: w.Stage.ApplicationID, RepoID: w.Run.ChartRepoID, InitiatorID: w.Run.InitiatorUserID, OperationID: w.Stage.OperationID, ChartName: w.Run.ChartName, ChartVersion: w.Run.ChartVersion, Digest: w.Run.ChartDigest, Purpose: fmt.Sprintf("delivery.run.%d.stage.%d", w.Run.ID, w.Stage.ID), Timeout: timeout}
}

func isAmbiguous(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	be, ok := apperr.AsBiz(err)
	if !ok {
		return true
	}
	switch be.Code {
	case apperr.CodeDeliveryApplicationUnavailable, apperr.CodeDeliveryChartIdentityMismatch, apperr.CodeDeliveryArtifactDrift:
		return false
	default:
		return true
	}
}

type gormStore struct{ db *gorm.DB }

func (s *gormStore) Claim(ctx context.Context, owner string, now time.Time, lease time.Duration) (*claimedWork, error) {
	var out *claimedWork
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Select and lock an eligible run first so every delivery mutation follows
		// run -> stage -> approval. EXISTS avoids locking runs without claimable
		// work, and both locks use SKIP LOCKED so competing workers never wait.
		var run models.DeliveryRun
		err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("state IN ? AND EXISTS (?)", []models.DeliveryRunState{models.DeliveryRunQueued, models.DeliveryRunRunning},
				tx.Model(&models.DeliveryRunStage{}).Select("1").Where("delivery_run_stages.run_id = delivery_runs.id AND delivery_run_stages.state = ? AND delivery_run_stages.executor = ? AND delivery_run_stages.lease_owner IS NULL", models.DeliveryStageQueued, models.DeliveryExecutorHelmUpgradeExistingRelease)).
			Order("id ASC").Limit(1).Take(&run).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		var stage models.DeliveryRunStage
		err = tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("run_id = ? AND state = ? AND executor = ? AND lease_owner IS NULL", run.ID, models.DeliveryStageQueued, models.DeliveryExecutorHelmUpgradeExistingRelease).Order("id ASC").Take(&stage).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		until := now.Add(lease)
		res := tx.Model(&models.DeliveryRunStage{}).Where("id = ? AND state = ? AND lease_owner IS NULL", stage.ID, models.DeliveryStageQueued).Updates(map[string]any{"state": models.DeliveryStageRunning, "lease_owner": owner, "lease_expires_at": until, "started_at": now, "updated_at": now})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return nil
		}
		if run.State == models.DeliveryRunQueued {
			old := run.State
			if err = transitionRun(tx, &run, models.DeliveryRunRunning, now, map[string]any{"started_at": now}); err != nil {
				return err
			}
			if err = appendTransition(tx, run.ID, nil, "run.running", old, models.DeliveryRunRunning, now, nil, nil); err != nil {
				return err
			}
		}
		stage.State = models.DeliveryStageRunning
		stage.LeaseOwner = &owner
		stage.LeaseExpiresAt = &until
		stage.StartedAt = &now
		if err = appendTransition(tx, run.ID, &stage.ID, "stage.running", models.DeliveryStageQueued, models.DeliveryStageRunning, now, nil, nil); err != nil {
			return err
		}
		out = &claimedWork{Run: run, Stage: stage}
		return nil
	})
	return out, err
}

func (s *gormStore) Renew(ctx context.Context, id uint64, owner string, until time.Time) (bool, bool, error) {
	var identity models.DeliveryRunStage
	if err := s.db.WithContext(ctx).Select("id", "run_id").First(&identity, id).Error; err != nil {
		return false, false, err
	}
	var renewed, cancelRequested bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run models.DeliveryRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, identity.RunID).Error; err != nil {
			return err
		}
		if run.State == models.DeliveryRunCancelRequested {
			cancelRequested = true
			return nil
		}
		res := tx.Model(&models.DeliveryRunStage{}).Where("id = ? AND state = ? AND lease_owner = ? AND lease_expires_at > CURRENT_TIMESTAMP", id, models.DeliveryStageRunning, owner).Updates(map[string]any{"lease_expires_at": until, "updated_at": time.Now().UTC()})
		if res.Error != nil {
			return res.Error
		}
		renewed = res.RowsAffected == 1
		return nil
	})
	return renewed, cancelRequested, err
}

func (s *gormStore) Complete(ctx context.Context, w claimedWork, owner string, now time.Time, c completion) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run models.DeliveryRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, w.Run.ID).Error; err != nil {
			return err
		}
		var stage models.DeliveryRunStage
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&stage, w.Stage.ID).Error; err != nil {
			return err
		}
		if stage.RunID != run.ID {
			return apperr.New(errs.CodeRunInvalidState, errs.KeyRunInvalidState, "delivery stage does not belong to run")
		}
		isOwner := stage.State == models.DeliveryStageRunning && stage.LeaseOwner != nil && *stage.LeaseOwner == owner
		if !isOwner {
			return nil
		}
		if stage.LeaseExpiresAt == nil || !stage.LeaseExpiresAt.After(now) {
			c.ambiguous = true
		}
		if run.State == models.DeliveryRunCancelRequested {
			c.ambiguous = true
		}
		if c.ambiguous {
			return s.reconcile(tx, &run, &stage, owner, now, c.intent)
		}
		if c.err != nil {
			return s.fail(tx, &run, &stage, now, c.err)
		}
		if c.result.Revision <= 0 || c.result.Digest == "" || c.result.Digest != run.ChartDigest {
			return s.reconcile(tx, &run, &stage, owner, now, c.intent)
		}
		return s.succeed(tx, &run, &stage, now, c.result)
	})
}

func (s *gormStore) reconcile(tx *gorm.DB, run *models.DeliveryRun, stage *models.DeliveryRunStage, _ string, now time.Time, intent recoveryIntent) error {
	if stage.State != models.DeliveryStageRunning {
		return nil
	}
	if err := transitionStage(tx, stage, models.DeliveryStageReconciling, now, map[string]any{"lease_owner": nil, "lease_expires_at": nil}); err != nil {
		return err
	}
	if run.State == models.DeliveryRunRunning || run.State == models.DeliveryRunCancelRequested {
		old := run.State
		if err := transitionRun(tx, run, models.DeliveryRunReconciling, now, nil); err != nil {
			return err
		}
		if err := appendTransition(tx, run.ID, nil, "run.reconciling", old, models.DeliveryRunReconciling, now, nil, nil); err != nil {
			return err
		}
	}
	intentName := map[recoveryIntent]string{recoveryFailed: "failed", recoveryCanceled: "canceled", recoveryTimedOut: "timed_out"}[intent]
	metadata, _ := json.Marshal(map[string]any{"recovery_intent": intentName})
	return appendTransition(tx, run.ID, &stage.ID, "stage.reconciling", models.DeliveryStageRunning, models.DeliveryStageReconciling, now, nil, nil, datatypes.JSON(metadata))
}

func (s *gormStore) fail(tx *gorm.DB, run *models.DeliveryRun, stage *models.DeliveryRunStage, now time.Time, cause error) error {
	be, ok := apperr.AsBiz(cause)
	if !ok {
		return s.reconcile(tx, run, stage, "", now, recoveryFailed)
	}
	code, key := int(be.Code), be.MessageKey
	if err := transitionStage(tx, stage, models.DeliveryStageFailed, now, map[string]any{"finished_at": now, "error_code": code, "error_message_key": key, "lease_owner": nil, "lease_expires_at": nil}); err != nil {
		return err
	}
	old := run.State
	if err := transitionRun(tx, run, models.DeliveryRunFailed, now, map[string]any{"finished_at": now, "error_code": code, "error_message_key": key}); err != nil {
		return err
	}
	if err := appendTransition(tx, run.ID, &stage.ID, "stage.failed", models.DeliveryStageRunning, models.DeliveryStageFailed, now, &code, &key); err != nil {
		return err
	}
	return appendTransition(tx, run.ID, nil, "run.failed", old, models.DeliveryRunFailed, now, &code, &key)
}

func (s *gormStore) succeed(tx *gorm.DB, run *models.DeliveryRun, stage *models.DeliveryRunStage, now time.Time, result UpgradeResult) error {
	if err := transitionStage(tx, stage, models.DeliveryStageSucceeded, now, map[string]any{"finished_at": now, "result_revision": result.Revision, "result_digest": result.Digest, "lease_owner": nil, "lease_expires_at": nil}); err != nil {
		return err
	}
	meta, _ := json.Marshal(map[string]any{"release_revision": result.Revision, "chart_digest": result.Digest})
	if err := appendTransition(tx, run.ID, &stage.ID, "stage.succeeded", models.DeliveryStageRunning, models.DeliveryStageSucceeded, now, nil, nil, datatypes.JSON(meta)); err != nil {
		return err
	}
	var next models.DeliveryRunStage
	err := tx.Where("run_id = ? AND stage_order > ?", run.ID, stage.StageOrder).Order("stage_order ASC,id ASC").First(&next).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		old := run.State
		if err = transitionRun(tx, run, models.DeliveryRunSucceeded, now, map[string]any{"finished_at": now}); err != nil {
			return err
		}
		return appendTransition(tx, run.ID, nil, "run.succeeded", old, models.DeliveryRunSucceeded, now, nil, nil)
	}
	if err != nil {
		return err
	}
	nextState := models.DeliveryStageQueued
	runState := models.DeliveryRunRunning
	if next.ApprovalRequired {
		nextState = models.DeliveryStageWaitingApproval
		runState = models.DeliveryRunWaitingApproval
		if err = tx.Create(&models.DeliveryApproval{RunID: run.ID, RunStageID: next.ID, RequestedAt: now, Decision: models.DeliveryApprovalPending, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
			return err
		}
	}
	if err = transitionStage(tx, &next, nextState, now, nil); err != nil {
		return err
	}
	oldRun := run.State
	if oldRun != runState {
		if err = transitionRun(tx, run, runState, now, nil); err != nil {
			return err
		}
	}
	if err = appendTransition(tx, run.ID, &next.ID, "stage."+string(nextState), models.DeliveryStagePending, nextState, now, nil, nil); err != nil {
		return err
	}
	if oldRun != runState {
		return appendTransition(tx, run.ID, nil, "run."+string(runState), oldRun, runState, now, nil, nil)
	}
	return nil
}

func transitionStage(tx *gorm.DB, row *models.DeliveryRunStage, to models.DeliveryStageState, now time.Time, extra map[string]any) error {
	if !deliveryrun.CanTransitionStage(row.State, to) {
		return apperr.New(errs.CodeRunInvalidState, errs.KeyRunInvalidState, "delivery stage transition is not permitted")
	}
	fields := map[string]any{"state": to, "updated_at": now}
	for k, v := range extra {
		fields[k] = v
	}
	res := tx.Model(&models.DeliveryRunStage{}).Where("id = ? AND state = ?", row.ID, row.State).Updates(fields)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return apperr.New(errs.CodeRunInvalidState, errs.KeyRunInvalidState, "delivery stage changed concurrently")
	}
	row.State = to
	return nil
}
func transitionRun(tx *gorm.DB, row *models.DeliveryRun, to models.DeliveryRunState, now time.Time, extra map[string]any) error {
	if !deliveryrun.CanTransitionRun(row.State, to) {
		return apperr.New(errs.CodeRunInvalidState, errs.KeyRunInvalidState, "delivery run transition is not permitted")
	}
	fields := map[string]any{"state": to, "updated_at": now}
	for k, v := range extra {
		fields[k] = v
	}
	res := tx.Model(&models.DeliveryRun{}).Where("id = ? AND state = ?", row.ID, row.State).Updates(fields)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return apperr.New(errs.CodeRunInvalidState, errs.KeyRunInvalidState, "delivery run changed concurrently")
	}
	row.State = to
	return nil
}
func appendTransition(tx *gorm.DB, runID uint64, stageID *uint64, event string, oldState, newState any, now time.Time, code *int, key *string, metadata ...datatypes.JSON) error {
	oldS, newS := fmt.Sprint(oldState), fmt.Sprint(newState)
	md := datatypes.JSON([]byte(`{}`))
	if len(metadata) > 0 {
		md = metadata[0]
	}
	return tx.Create(&models.DeliveryRunEvent{RunID: runID, RunStageID: stageID, EventType: event, OldState: &oldS, NewState: &newS, ActorType: models.DeliveryEventActorSystem, OccurredAt: now, ErrorCode: code, ErrorMessageKey: key, Metadata: md}).Error
}
