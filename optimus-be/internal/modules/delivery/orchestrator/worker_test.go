package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
)

type fakeExecutor struct {
	mu            sync.Mutex
	calls         []UpgradeRequest
	result        UpgradeResult
	err           error
	block         chan struct{}
	ignoreContext bool
}

func (f *fakeExecutor) UpgradeExisting(ctx context.Context, req UpgradeRequest) (UpgradeResult, error) {
	f.mu.Lock()
	f.calls = append(f.calls, req)
	f.mu.Unlock()
	if f.block != nil {
		if f.ignoreContext {
			<-f.block
			return f.result, f.err
		}
		select {
		case <-f.block:
		case <-ctx.Done():
			return UpgradeResult{}, ctx.Err()
		}
	}
	return f.result, f.err
}

type fakeClock struct {
	now   time.Time
	ticks chan time.Time
}

func (f fakeClock) Now() time.Time                 { return f.now }
func (f fakeClock) NewTicker(time.Duration) ticker { return fakeTicker{f.ticks} }

type fakeTicker struct{ ticks chan time.Time }

func (f fakeTicker) Chan() <-chan time.Time { return f.ticks }
func (fakeTicker) Stop()                    {}

type fakeStore struct {
	mu            sync.Mutex
	works         []claimedWork
	completions   []completion
	renews        int
	loseLease     bool
	claimErr      error
	renewErr      error
	completeErr   error
	blockRenew    bool
	blockComplete bool
}

func (f *fakeStore) Claim(_ context.Context, _ string, _ time.Time, _ time.Duration) (*claimedWork, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.claimErr != nil {
		return nil, f.claimErr
	}
	if len(f.works) == 0 {
		return nil, nil
	}
	w := f.works[0]
	f.works = f.works[1:]
	return &w, nil
}
func (f *fakeStore) Renew(ctx context.Context, _ uint64, _ string, _ time.Time) (bool, error) {
	f.mu.Lock()
	f.renews++
	block, ok, err := f.blockRenew, !f.loseLease, f.renewErr
	f.mu.Unlock()
	if block {
		<-ctx.Done()
		return false, ctx.Err()
	}
	return ok, err
}
func (f *fakeStore) Complete(ctx context.Context, _ claimedWork, _ string, _ time.Time, c completion) error {
	f.mu.Lock()
	f.completions = append(f.completions, c)
	block, err := f.blockComplete, f.completeErr
	f.mu.Unlock()
	if block {
		<-ctx.Done()
		return ctx.Err()
	}
	return err
}

func work(id uint64) claimedWork {
	return claimedWork{Run: models.DeliveryRun{ID: id, ChartRepoID: 2, ChartName: "app", ChartVersion: "1", ChartDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", InitiatorUserID: 3}, Stage: models.DeliveryRunStage{ID: id, ApplicationID: 10, OperationID: "delivery-op", TimeoutSeconds: 60, Executor: models.DeliveryExecutorHelmUpgradeExistingRelease}}
}
func config() Config {
	return Config{Concurrency: 1, LeaseDuration: time.Minute, RenewInterval: 10 * time.Millisecond, PollInterval: time.Millisecond, PersistenceTimeout: 50 * time.Millisecond}
}

func TestWorkerBuildsClosedUpgradeRequestAndCompletes(t *testing.T) {
	s := &fakeStore{works: []claimedWork{work(1)}}
	e := &fakeExecutor{result: UpgradeResult{Revision: 7, Digest: work(1).Run.ChartDigest}}
	w := newWorker(s, e, config(), "owner", fakeClock{now: time.Unix(10, 0).UTC(), ticks: make(chan time.Time)})
	require.NoError(t, w.ProcessOnce(context.Background()))
	require.Len(t, e.calls, 1)
	require.Equal(t, "delivery.run.1.stage.1", e.calls[0].Purpose)
	require.Equal(t, "delivery-op", e.calls[0].OperationID)
	require.Len(t, s.completions, 1)
	require.False(t, s.completions[0].ambiguous)
}
func TestWorkerClassifiesDefiniteAndAmbiguousFailures(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		ambiguous bool
	}{
		{"application unavailable", apperr.New(apperr.CodeDeliveryApplicationUnavailable, "delivery.application.unavailable", "safe"), false},
		{"chart identity mismatch", apperr.New(apperr.CodeDeliveryChartIdentityMismatch, "delivery.execution.chart_identity_mismatch", "safe"), false},
		{"artifact drift", apperr.New(apperr.CodeDeliveryArtifactDrift, "delivery.execution.artifact_drift", "safe"), false},
		{"transport", errors.New("raw secret"), true},
		{"operation busy", apperr.New(apperr.CodeDeliveryOperationBusy, "delivery.execution.operation_busy", "safe"), true},
		{"reconcile", apperr.New(apperr.CodeDeliveryReconciliationRequired, "delivery.execution.reconciliation_required", "safe"), true},
		{"outcome unknown", apperr.New(apperr.CodeDeliveryOutcomeUnknown, "delivery.execution.outcome_unknown", "safe"), true},
		{"timeout", apperr.New(apperr.CodeDeliveryExecutionTimeout, "delivery.execution.timeout", "safe"), true},
		{"unavailable", apperr.New(apperr.CodeDeliveryExecutionUnavailable, "delivery.execution.unavailable", "safe"), true},
		{"future business error", apperr.New(apperr.CodeDeliveryRunInvalidState, "delivery.run.invalid_state", "safe"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &fakeStore{works: []claimedWork{work(1)}}
			w := newWorker(s, &fakeExecutor{err: tc.err}, config(), "owner", fakeClock{now: time.Now(), ticks: make(chan time.Time)})
			require.NoError(t, w.ProcessOnce(context.Background()))
			require.Equal(t, tc.ambiguous, s.completions[0].ambiguous)
		})
	}
}
func TestWorkerRenewsLeasePeriodically(t *testing.T) {
	s := &fakeStore{works: []claimedWork{work(1)}}
	block := make(chan struct{})
	ticks := make(chan time.Time, 2)
	w := newWorker(s, &fakeExecutor{block: block}, config(), "owner", fakeClock{now: time.Now(), ticks: ticks})
	done := make(chan struct{})
	go func() { _ = w.ProcessOnce(context.Background()); close(done) }()
	ticks <- time.Now()
	ticks <- time.Now()
	require.Eventually(t, func() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.renews >= 2 }, time.Second, time.Millisecond)
	close(block)
	<-done
}
func TestWorkerLeaseLossForcesReconciliation(t *testing.T) {
	s := &fakeStore{works: []claimedWork{work(1)}, loseLease: true}
	ticks := make(chan time.Time, 1)
	w := newWorker(s, &fakeExecutor{block: make(chan struct{})}, config(), "owner", fakeClock{now: time.Now(), ticks: ticks})
	ticks <- time.Now()
	require.NoError(t, w.ProcessOnce(context.Background()))
	require.True(t, s.completions[0].ambiguous)
}

func TestWorkerLeaseLossDoesNotWaitForUnresponsiveExecutor(t *testing.T) {
	s := &fakeStore{works: []claimedWork{work(1)}, loseLease: true}
	ticks := make(chan time.Time, 1)
	release := make(chan struct{})
	w := newWorker(s, &fakeExecutor{block: release, ignoreContext: true}, config(), "owner", fakeClock{now: time.Now(), ticks: ticks})
	done := make(chan error, 1)
	go func() { done <- w.ProcessOnce(context.Background()) }()
	ticks <- time.Now()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("worker waited for an executor that ignored cancellation")
	}
	close(release)
	require.True(t, s.completions[0].ambiguous)
}

func TestWorkerPropagatesStoreErrors(t *testing.T) {
	t.Run("claim", func(t *testing.T) {
		wanted := errors.New("claim failed")
		w := newWorker(&fakeStore{claimErr: wanted}, &fakeExecutor{}, config(), "owner", fakeClock{ticks: make(chan time.Time)})
		require.ErrorIs(t, w.ProcessOnce(context.Background()), wanted)
	})
	t.Run("renewal completion", func(t *testing.T) {
		wanted := errors.New("complete failed")
		ticks := make(chan time.Time, 1)
		release := make(chan struct{})
		s := &fakeStore{works: []claimedWork{work(1)}, renewErr: errors.New("renew failed"), completeErr: wanted}
		w := newWorker(s, &fakeExecutor{block: release, ignoreContext: true}, config(), "owner", fakeClock{now: time.Now(), ticks: ticks})
		done := make(chan error, 1)
		go func() { done <- w.ProcessOnce(context.Background()) }()
		ticks <- time.Now()
		require.ErrorIs(t, <-done, wanted)
		close(release)
	})
}

func TestWorkerBoundsPersistenceAfterCancellationAndLeaseLoss(t *testing.T) {
	t.Run("completion", func(t *testing.T) {
		cfg := config()
		cfg.PersistenceTimeout = 20 * time.Millisecond
		s := &fakeStore{works: []claimedWork{work(1)}, blockComplete: true}
		w := newWorker(s, &fakeExecutor{result: UpgradeResult{Revision: 7, Digest: work(1).Run.ChartDigest}}, cfg, "owner", fakeClock{now: time.Now(), ticks: make(chan time.Time)})
		started := time.Now()
		err := w.ProcessOnce(context.Background())
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Less(t, time.Since(started), time.Second)
	})

	t.Run("renewal and reconciliation completion", func(t *testing.T) {
		cfg := config()
		cfg.PersistenceTimeout = 20 * time.Millisecond
		ticks := make(chan time.Time, 1)
		release := make(chan struct{})
		s := &fakeStore{works: []claimedWork{work(1)}, blockRenew: true, blockComplete: true}
		w := newWorker(s, &fakeExecutor{block: release, ignoreContext: true}, cfg, "owner", fakeClock{now: time.Now(), ticks: ticks})
		done := make(chan error, 1)
		go func() { done <- w.ProcessOnce(context.Background()) }()
		ticks <- time.Now()
		select {
		case err := <-done:
			require.ErrorIs(t, err, context.DeadlineExceeded)
		case <-time.After(time.Second):
			t.Fatal("bounded persistence contexts did not return")
		}
		close(release)
	})
}

func TestWorkerDetachedExecutorKeepsConcurrencySlot(t *testing.T) {
	s := &fakeStore{works: []claimedWork{work(1), work(2)}}
	release := make(chan struct{})
	e := &fakeExecutor{block: release, ignoreContext: true}
	w := newWorker(s, e, config(), "owner", realClock{})

	firstCtx, cancelFirst := context.WithCancel(context.Background())
	firstDone := make(chan struct{})
	go func() { w.Run(firstCtx); close(firstDone) }()
	require.Eventually(t, func() bool { e.mu.Lock(); defer e.mu.Unlock(); return len(e.calls) == 1 }, time.Second, time.Millisecond)
	cancelFirst()
	require.Eventually(t, func() bool {
		select {
		case <-firstDone:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond)

	secondCtx, cancelSecond := context.WithCancel(context.Background())
	secondDone := make(chan struct{})
	go func() { w.Run(secondCtx); close(secondDone) }()
	time.Sleep(25 * time.Millisecond)
	e.mu.Lock()
	require.Len(t, e.calls, 1, "detached execution must retain its semaphore slot")
	e.mu.Unlock()
	close(release)
	require.Eventually(t, func() bool { e.mu.Lock(); defer e.mu.Unlock(); return len(e.calls) == 2 }, time.Second, time.Millisecond)
	cancelSecond()
	require.Eventually(t, func() bool {
		select {
		case <-secondDone:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond)
}
func TestWorkerBoundedConcurrencyAndGracefulStop(t *testing.T) {
	s := &fakeStore{works: []claimedWork{work(1), work(2)}}
	block := make(chan struct{})
	e := &fakeExecutor{block: block}
	w := newWorker(s, e, config(), "owner", realClock{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()
	require.Eventually(t, func() bool { e.mu.Lock(); defer e.mu.Unlock(); return len(e.calls) == 1 }, time.Second, time.Millisecond)
	cancel()
	close(block)
	require.Eventually(t, func() bool {
		select {
		case <-done:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond)
	e.mu.Lock()
	defer e.mu.Unlock()
	require.Len(t, e.calls, 1)
}
