package module

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/robfig/cron/v3"
	"github.com/stretchr/testify/require"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/response"
	"optimus-be/internal/modules/assets/account"
	asseterrs "optimus-be/internal/modules/assets/errs"
)

type fakeAccountSyncService struct {
	detail     *account.Detail
	err        error
	auditCalls int
	auditActor *uint64
}

func (f *fakeAccountSyncService) Get(context.Context, uint64) (*account.Detail, error) {
	return f.detail, f.err
}

func (f *fakeAccountSyncService) RecordSyncTrigger(_ context.Context, actor *uint64, _, _ string, _ uint64) {
	f.auditCalls++
	f.auditActor = actor
}

type fakeSyncEngine struct {
	locked      bool
	runStarted  chan context.Context
	runReleased chan struct{}
	waitCancel  bool
	unlockCalls atomic.Int32
}

func (f *fakeSyncEngine) TryLock(uint64) bool { return f.locked }
func (f *fakeSyncEngine) Unlock(uint64)       { f.unlockCalls.Add(1) }
func (f *fakeSyncEngine) RunAccountLocked(ctx context.Context, _ uint64, _ string, _ *uint64) error {
	f.runStarted <- ctx
	if f.waitCancel {
		<-ctx.Done()
		return ctx.Err()
	}
	if f.runReleased != nil {
		<-f.runReleased
	}
	return nil
}

func TestModuleShutdown_CancelsAndWaitsForManualWorker(t *testing.T) {
	rootCtx, cancel := context.WithCancel(context.Background())
	m := &Module{cancel: cancel, CronScheduler: cron.New()}
	svc := &fakeAccountSyncService{detail: enabledAccount("us-east-1")}
	engine := &fakeSyncEngine{locked: true, runStarted: make(chan context.Context, 1), waitCancel: true}
	c, rec, _ := testContext(t)

	newManualSyncTrigger(rootCtx, svc, engine, time.Hour, nil, &m.workers)(c, 42)
	require.Equal(t, http.StatusOK, rec.Code)
	var workerCtx context.Context
	select {
	case workerCtx = <-engine.runStarted:
	case <-time.After(time.Second):
		t.Fatal("manual sync worker did not start")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutdownCancel()
	require.NoError(t, m.Shutdown(shutdownCtx))
	require.ErrorIs(t, workerCtx.Err(), context.Canceled)
	require.EqualValues(t, 1, engine.unlockCalls.Load())

	// Once shutdown begins, no later trigger can add work or retain the lock.
	lateEngine := &fakeSyncEngine{locked: true, runStarted: make(chan context.Context, 1)}
	lateCtx, lateRec, _ := testContext(t)
	newManualSyncTrigger(rootCtx, svc, lateEngine, time.Hour, nil, &m.workers)(lateCtx, 42)
	require.Equal(t, http.StatusInternalServerError, lateRec.Code)
	require.EqualValues(t, 1, lateEngine.unlockCalls.Load())
	require.Empty(t, lateEngine.runStarted)
}

func TestManualSyncTrigger_RejectsDisabledAccountBeforeLockAndAudit(t *testing.T) {
	svc := &fakeAccountSyncService{detail: &account.Detail{Summary: account.Summary{Enabled: false}}}
	engine := &fakeSyncEngine{locked: true, runStarted: make(chan context.Context, 1)}
	c, rec, _ := testContext(t)

	newManualSyncTrigger(context.Background(), svc, engine, time.Second, nil, &workerGroup{})(c, 42)

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Equal(t, int(asseterrs.CodeAssetsCloudAccountDisabled), envelopeCode(t, rec))
	require.Zero(t, svc.auditCalls)
	require.Empty(t, engine.runStarted)
}

func TestManualSyncTrigger_RejectsBusyAccountWithoutAudit(t *testing.T) {
	svc := &fakeAccountSyncService{detail: enabledAccount("us-east-1")}
	engine := &fakeSyncEngine{locked: false, runStarted: make(chan context.Context, 1)}
	c, rec, _ := testContext(t)

	newManualSyncTrigger(context.Background(), svc, engine, time.Second, nil, &workerGroup{})(c, 42)

	require.Equal(t, http.StatusConflict, rec.Code)
	require.Equal(t, int(asseterrs.CodeAssetsSyncBusy), envelopeCode(t, rec))
	require.Zero(t, svc.auditCalls)
}

func TestManualSyncTrigger_AuditsInlineAndRunsWithDetachedBoundedContext(t *testing.T) {
	svc := &fakeAccountSyncService{detail: enabledAccount("us-east-1", "eu-west-1")}
	release := make(chan struct{})
	engine := &fakeSyncEngine{locked: true, runStarted: make(chan context.Context, 1), runReleased: release}
	c, rec, cancelRequest := testContext(t)
	c.Set(middleware.CtxKeyUserID, uint64(7))

	newManualSyncTrigger(context.Background(), svc, engine, 50*time.Millisecond, nil, &workerGroup{})(c, 42)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, int(apperr.CodeOK), envelopeCode(t, rec))
	require.Equal(t, 1, svc.auditCalls)
	require.NotNil(t, svc.auditActor)
	require.Equal(t, uint64(7), *svc.auditActor)

	cancelRequest()
	var workerCtx context.Context
	select {
	case workerCtx = <-engine.runStarted:
	case <-time.After(time.Second):
		t.Fatal("manual sync worker did not start")
	}
	require.NoError(t, workerCtx.Err(), "request cancellation must not cancel the worker")
	deadline, ok := workerCtx.Deadline()
	require.True(t, ok)
	require.WithinDuration(t, time.Now().Add(300*time.Millisecond), deadline, 100*time.Millisecond)

	close(release)
	require.Eventually(t, func() bool { return engine.unlockCalls.Load() == 1 }, time.Second, time.Millisecond)
}

func TestManualSyncTrigger_PropagatesLookupError(t *testing.T) {
	want := apperr.New(apperr.CodeNotFound, "common.not_found", "not found")
	svc := &fakeAccountSyncService{err: want}
	engine := &fakeSyncEngine{locked: true, runStarted: make(chan context.Context, 1)}
	c, rec, _ := testContext(t)

	newManualSyncTrigger(context.Background(), svc, engine, time.Second, nil, &workerGroup{})(c, 42)

	require.Equal(t, apperr.HTTPStatus(apperr.CodeNotFound), rec.Code)
	require.Equal(t, int(apperr.CodeNotFound), envelopeCode(t, rec))
}

func enabledAccount(regions ...string) *account.Detail {
	return &account.Detail{Summary: account.Summary{Enabled: true}, EnabledRegions: regions}
}

func testContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder, context.CancelFunc) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	requestCtx, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil).WithContext(requestCtx)
	return c, rec, cancel
}

func envelopeCode(t *testing.T, rec *httptest.ResponseRecorder) int {
	t.Helper()
	var got response.Envelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	return got.Code
}
