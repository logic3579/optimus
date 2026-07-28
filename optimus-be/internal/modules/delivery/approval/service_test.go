package approval

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
)

func TestServiceListPendingReturnsOnlyActorsActionableQueue(t *testing.T) {
	requested := time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC)
	repo := &fakeRepository{pending: []PendingRow{{
		ApprovalID: 11, RunID: 12, RunStageID: 13, ProjectID: 14,
		ProjectName: "payments", EnvironmentKey: "prod", EnvironmentName: "Production",
		StageOrder: 2, ChartName: "payments", ChartVersion: "1.2.3",
		ChartDigest:     "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		InitiatorUserID: 7, RequestedAt: requested,
	}}}
	svc := NewService(repo, nil, nil)

	items, err := svc.ListPending(context.Background(), 42)

	require.NoError(t, err)
	require.Equal(t, uint64(42), repo.pendingActor)
	require.Equal(t, []PendingApproval{{
		ID: 11, RunID: 12, RunStageID: 13, ProjectID: 14,
		ProjectName: "payments", EnvironmentKey: "prod", EnvironmentName: "Production",
		StageOrder: 2, ChartName: "payments", ChartVersion: "1.2.3",
		ChartDigest:     "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		InitiatorUserID: 7, RequestedAt: requested,
	}}, items)
}

func TestServiceApproveRequiresLiveDecisionPermission(t *testing.T) {
	steps := []string{}
	repo := decisionFixture(7)
	repo.steps = &steps
	checker := &fakePermissionChecker{allowed: false, steps: &steps}
	recorder := &fakeRecorder{}
	svc := NewService(repo, checker, recorder)

	_, err := svc.Approve(context.Background(), 42, "127.0.0.1", "agent", repo.stage.ID, DecisionRequest{Comment: "Reviewed"})

	require.Error(t, err)
	var biz *apperr.BizError
	require.True(t, errors.As(err, &biz))
	require.Equal(t, apperr.CodePermissionDenied, biz.Code)
	require.Equal(t, "auth.permission_denied", biz.MessageKey)
	require.Equal(t, uint64(42), checker.actor)
	require.Equal(t, "delivery:approval:decide", checker.code)
	require.Equal(t, []string{"lock", "permission"}, steps)
	require.Zero(t, repo.decisionCalls)
	require.Zero(t, repo.stageCalls)
	require.Zero(t, repo.runCalls)
	require.Empty(t, repo.events)
	require.Empty(t, recorder.events)
	require.Equal(t, models.DeliveryApprovalPending, repo.approval.Decision)
	require.Equal(t, models.DeliveryStageWaitingApproval, repo.stage.State)
	require.Equal(t, models.DeliveryRunWaitingApproval, repo.run.State)
}

func TestServicePermissionProviderErrorAfterLockReturnsSafeCause(t *testing.T) {
	steps := []string{}
	cause := errors.New("sensitive permission provider failure")
	repo := decisionFixture(7)
	repo.steps = &steps
	checker := &fakePermissionChecker{err: cause, steps: &steps}
	recorder := &fakeRecorder{}
	svc := NewService(repo, checker, recorder)

	_, err := svc.Approve(context.Background(), 42, "", "", repo.stage.ID, DecisionRequest{Comment: "Reviewed"})

	require.ErrorIs(t, err, cause)
	var biz *apperr.BizError
	require.ErrorAs(t, err, &biz)
	require.Equal(t, apperr.CodeInternal, biz.Code)
	require.Equal(t, "common.internal", biz.MessageKey)
	require.NotContains(t, biz.Message, cause.Error())
	require.Equal(t, []string{"lock", "permission"}, steps)
	require.ErrorIs(t, repo.transactionErr, cause)
	require.Zero(t, repo.decisionCalls)
	require.Zero(t, repo.stageCalls)
	require.Zero(t, repo.runCalls)
	require.Empty(t, repo.events)
	require.Empty(t, recorder.events)
}

func TestServiceDecisionRequiresBoundedComment(t *testing.T) {
	tests := []struct {
		name    string
		comment string
		code    apperr.Code
	}{
		{name: "empty", comment: " \t\n ", code: apperr.CodeDeliveryApprovalCommentRequired},
		{name: "too long", comment: strings.Repeat("x", 513), code: apperr.CodeDeliveryApprovalCommentInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewService(&fakeRepository{}, &fakePermissionChecker{allowed: true}, nil)

			_, err := svc.Approve(context.Background(), 42, "", "", 13, DecisionRequest{Comment: tt.comment})

			var biz *apperr.BizError
			require.ErrorAs(t, err, &biz)
			require.Equal(t, tt.code, biz.Code)
		})
	}
}

func TestServiceApproveRejectsRunInitiator(t *testing.T) {
	repo := decisionFixture(42)
	svc := NewService(repo, &fakePermissionChecker{allowed: true}, nil)

	_, err := svc.Approve(context.Background(), 42, "", "", repo.stage.ID, DecisionRequest{Comment: "Looks good"})

	var biz *apperr.BizError
	require.ErrorAs(t, err, &biz)
	require.Equal(t, apperr.CodeDeliveryApprovalSelfApproval, biz.Code)
	require.Equal(t, models.DeliveryApprovalPending, repo.approval.Decision)
	require.Equal(t, models.DeliveryStageWaitingApproval, repo.stage.State)
	require.Equal(t, models.DeliveryRunWaitingApproval, repo.run.State)
}

func TestServiceApproveQueuesStageAndRunWithRedactedRecords(t *testing.T) {
	repo := decisionFixture(7)
	recorder := &fakeRecorder{}
	svc := NewService(repo, &fakePermissionChecker{allowed: true}, recorder)
	decidedAt := time.Date(2026, 7, 28, 11, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return decidedAt }

	got, err := svc.Approve(context.Background(), 42, "127.0.0.1", "agent", repo.stage.ID, DecisionRequest{Comment: "  Reviewed safely  "})

	require.NoError(t, err)
	require.Equal(t, &Decision{
		ID: repo.approval.ID, RunID: repo.run.ID, RunStageID: repo.stage.ID,
		Decision: models.DeliveryApprovalApproved, DecidedByUserID: 42,
		Comment: "Reviewed safely", DecidedAt: decidedAt,
	}, got)
	require.Equal(t, models.DeliveryStageQueued, repo.stage.State)
	require.Equal(t, models.DeliveryRunQueued, repo.run.State)
	require.Len(t, repo.events, 1)
	require.Equal(t, "stage.approved", repo.events[0].EventType)
	require.NotContains(t, string(repo.events[0].Metadata), "Reviewed safely")
	require.Len(t, recorder.events, 1)
	event := recorder.events[0]
	require.Equal(t, "delivery.approval.approve", event.Action)
	payload := event.Payload.(map[string]any)
	require.Equal(t, "approved", payload["decision"])
	require.Equal(t, true, payload["comment_present"])
	sum := sha256.Sum256([]byte("Reviewed safely"))
	require.Equal(t, hex.EncodeToString(sum[:]), payload["comment_sha256"])
	require.NotContains(t, payload, "comment")
}

func TestServiceRejectTerminatesStageAndRun(t *testing.T) {
	repo := decisionFixture(7)
	recorder := &fakeRecorder{}
	svc := NewService(repo, &fakePermissionChecker{allowed: true}, recorder)
	decidedAt := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return decidedAt }

	got, err := svc.Reject(context.Background(), 42, "", "", repo.stage.ID, DecisionRequest{Comment: "Risk is too high"})

	require.NoError(t, err)
	require.Equal(t, models.DeliveryApprovalRejected, got.Decision)
	require.Equal(t, models.DeliveryStageRejected, repo.stage.State)
	require.Equal(t, models.DeliveryRunRejected, repo.run.State)
	require.Equal(t, decidedAt, *repo.stage.FinishedAt)
	require.Equal(t, decidedAt, *repo.run.FinishedAt)
	require.Equal(t, "stage.rejected", repo.events[0].EventType)
	require.Equal(t, "delivery.approval.reject", recorder.events[0].Action)
}

func TestServiceDecisionReplayAndConflicts(t *testing.T) {
	repo := decisionFixture(7)
	recorder := &fakeRecorder{}
	checker := &fakePermissionChecker{allowed: true}
	svc := NewService(repo, checker, recorder)
	svc.now = func() time.Time { return time.Date(2026, 7, 28, 13, 0, 0, 0, time.UTC) }
	req := DecisionRequest{Comment: "Reviewed"}

	first, err := svc.Approve(context.Background(), 42, "", "", repo.stage.ID, req)
	require.NoError(t, err)
	checker.allowed = false
	_, err = svc.Approve(context.Background(), 42, "", "", repo.stage.ID, req)
	requireBizCode(t, err, apperr.CodePermissionDenied)
	checker.allowed = true
	replayed, err := svc.Approve(context.Background(), 42, "", "", repo.stage.ID, req)
	require.NoError(t, err)
	require.Equal(t, first, replayed)
	require.Len(t, repo.events, 1)
	require.Len(t, recorder.events, 1)

	_, err = svc.Approve(context.Background(), 42, "", "", repo.stage.ID, DecisionRequest{Comment: "Changed"})
	requireBizCode(t, err, apperr.CodeDeliveryApprovalAlreadyDecided)
	_, err = svc.Reject(context.Background(), 42, "", "", repo.stage.ID, req)
	requireBizCode(t, err, apperr.CodeDeliveryApprovalDecisionConflict)
}

func requireBizCode(t *testing.T, err error, code apperr.Code) {
	t.Helper()
	var biz *apperr.BizError
	require.ErrorAs(t, err, &biz)
	require.Equal(t, code, biz.Code)
}

type fakeRepository struct {
	pending        []PendingRow
	pendingActor   uint64
	approval       models.DeliveryApproval
	run            models.DeliveryRun
	stage          models.DeliveryRunStage
	events         []models.DeliveryRunEvent
	steps          *[]string
	decisionCalls  int
	stageCalls     int
	runCalls       int
	transactionErr error
}

func (r *fakeRepository) ListPending(_ context.Context, actor uint64) ([]PendingRow, error) {
	r.pendingActor = actor
	return r.pending, nil
}

func (r *fakeRepository) Transaction(_ context.Context, fn func(repository) error) error {
	r.transactionErr = fn(r)
	return r.transactionErr
}

func (r *fakeRepository) LockForDecision(_ context.Context, _ uint64) (*models.DeliveryApproval, *models.DeliveryRunStage, *models.DeliveryRun, error) {
	if r.steps != nil {
		*r.steps = append(*r.steps, "lock")
	}
	return &r.approval, &r.stage, &r.run, nil
}

func (r *fakeRepository) DecideApproval(_ context.Context, _ uint64, actor uint64, decision models.DeliveryApprovalDecision, comment string, now time.Time) error {
	r.decisionCalls++
	r.approval.Decision, r.approval.DecidedByUserID, r.approval.Comment = decision, &actor, comment
	r.approval.DecidedAt, r.approval.UpdatedAt = &now, now
	return nil
}

func (r *fakeRepository) TransitionStage(_ context.Context, _ uint64, from, to models.DeliveryStageState, _ time.Time, finishedAt *time.Time) error {
	r.stageCalls++
	if r.stage.State != from {
		return errors.New("stage state conflict")
	}
	r.stage.State, r.stage.FinishedAt = to, finishedAt
	return nil
}

func (r *fakeRepository) TransitionRun(_ context.Context, _ uint64, from, to models.DeliveryRunState, _ time.Time, finishedAt *time.Time) error {
	r.runCalls++
	if r.run.State != from {
		return errors.New("run state conflict")
	}
	r.run.State, r.run.FinishedAt = to, finishedAt
	return nil
}

func (r *fakeRepository) AppendEvent(_ context.Context, event *models.DeliveryRunEvent) error {
	r.events = append(r.events, *event)
	return nil
}

func decisionFixture(initiator uint64) *fakeRepository {
	now := time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC)
	return &fakeRepository{
		approval: models.DeliveryApproval{ID: 3, RunID: 5, RunStageID: 7, RequestedAt: now, Decision: models.DeliveryApprovalPending},
		run:      models.DeliveryRun{ID: 5, InitiatorUserID: initiator, State: models.DeliveryRunWaitingApproval},
		stage:    models.DeliveryRunStage{ID: 7, RunID: 5, State: models.DeliveryStageWaitingApproval},
	}
}

type fakePermissionChecker struct {
	allowed bool
	err     error
	actor   uint64
	code    string
	steps   *[]string
}

func (c *fakePermissionChecker) Has(_ context.Context, actor uint64, code string) (bool, error) {
	if c.steps != nil {
		*c.steps = append(*c.steps, "permission")
	}
	c.actor, c.code = actor, code
	return c.allowed, c.err
}

type fakeRecorder struct{ events []audit.Event }

func (r *fakeRecorder) Record(_ context.Context, event audit.Event) error {
	r.events = append(r.events, event)
	return nil
}
