package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	"optimus-be/internal/models"
	"optimus-be/internal/modules/delivery/errs"
)

type reconcileMemoryStore struct {
	candidate         *reconcileCandidate
	outcomes          []reconcileOutcome
	resolveContextErr error
}

func TestRecoveryIntentSurvivesManualReconciliationEvent(t *testing.T) {
	timedOut, err := json.Marshal(map[string]any{"recovery_intent": "timed_out"})
	require.NoError(t, err)
	events := []models.DeliveryRunEvent{
		{ID: 9, Metadata: datatypes.JSON([]byte(`{}`))},
		{ID: 8, Metadata: datatypes.JSON(timedOut)},
	}
	require.Equal(t, recoveryTimedOut, recoveryIntentFromEvents(events))
}

func (s *reconcileMemoryStore) Load(context.Context, uint64, time.Time) (*reconcileCandidate, error) {
	return s.candidate, nil
}
func (s *reconcileMemoryStore) Resolve(ctx context.Context, _ reconcileCandidate, outcome reconcileOutcome, _ time.Time) error {
	s.resolveContextErr = ctx.Err()
	s.outcomes = append(s.outcomes, outcome)
	return nil
}

type inspectionStub struct {
	evidence    Inspection
	err         error
	application uint64
	operation   string
	cancel      context.CancelFunc
}

func (s *inspectionStub) Inspect(_ context.Context, application uint64, operation string) (Inspection, error) {
	s.application, s.operation = application, operation
	if s.cancel != nil {
		s.cancel()
	}
	return s.evidence, s.err
}

func TestReconcileUsesOnlyDefiniteInspectionEvidence(t *testing.T) {
	target := "sha256:" + strings.Repeat("a", 64)
	tests := []struct {
		name       string
		intent     recoveryIntent
		evidence   Inspection
		inspectErr error
		run        models.DeliveryRunState
		stage      models.DeliveryStageState
		drift      bool
	}{
		{"target digest succeeds", recoveryFailed, Inspection{Revision: 8, Digest: target}, nil, models.DeliveryRunSucceeded, models.DeliveryStageSucceeded, false},
		{"cancel previous digest proven", recoveryCanceled, Inspection{Revision: 7, Digest: "previous", PreviousDigestProven: true}, nil, models.DeliveryRunCanceled, models.DeliveryStageCanceled, false},
		{"timeout previous digest proven", recoveryTimedOut, Inspection{Revision: 7, Digest: "previous", PreviousDigestProven: true}, nil, models.DeliveryRunTimedOut, models.DeliveryStageTimedOut, false},
		{"failure previous digest proven", recoveryFailed, Inspection{Revision: 7, Digest: "previous", PreviousDigestProven: true}, nil, models.DeliveryRunFailed, models.DeliveryStageFailed, false},
		{"proof without previous observation unknown", recoveryCanceled, Inspection{PreviousDigestProven: true}, nil, models.DeliveryRunOutcomeUnknown, models.DeliveryStageOutcomeUnknown, false},
		{"non target digest alone unknown", recoveryTimedOut, Inspection{Revision: 7, Digest: "previous"}, nil, models.DeliveryRunOutcomeUnknown, models.DeliveryStageOutcomeUnknown, true},
		{"inspection error discards partial evidence", recoveryFailed, Inspection{Revision: 99, Digest: target, PreviousDigestProven: true}, errors.New("upstream secret"), models.DeliveryRunOutcomeUnknown, models.DeliveryStageOutcomeUnknown, false},
		{"rollback drift unknown", recoveryFailed, Inspection{Revision: 9, Digest: "rollback"}, nil, models.DeliveryRunOutcomeUnknown, models.DeliveryStageOutcomeUnknown, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &reconcileMemoryStore{candidate: &reconcileCandidate{Run: models.DeliveryRun{ID: 1, ChartDigest: target}, Stage: models.DeliveryRunStage{ID: 2, ApplicationID: 3, OperationID: "op"}, Intent: tt.intent}}
			inspector := &inspectionStub{evidence: tt.evidence, err: tt.inspectErr}
			r := newReconciler(store, inspector, func() time.Time { return time.Unix(10, 0) })
			require.NoError(t, r.Reconcile(context.Background(), 1))
			require.Len(t, store.outcomes, 1)
			require.Equal(t, tt.run, store.outcomes[0].RunState)
			require.Equal(t, tt.stage, store.outcomes[0].StageState)
			require.Equal(t, tt.drift, store.outcomes[0].Drift)
			if tt.inspectErr != nil {
				require.Zero(t, store.outcomes[0].Revision)
				require.Empty(t, store.outcomes[0].Digest)
			}
			if tt.run == models.DeliveryRunOutcomeUnknown {
				require.Equal(t, int(errs.CodeOutcomeUnknown), *store.outcomes[0].ErrorCode)
				require.Equal(t, errs.KeyOutcomeUnknown, *store.outcomes[0].ErrorKey)
			}
			if tt.run == models.DeliveryRunTimedOut {
				require.Equal(t, int(errs.CodeExecutionTimeout), *store.outcomes[0].ErrorCode)
				require.Equal(t, errs.KeyExecutionTimeout, *store.outcomes[0].ErrorKey)
			}
			if tt.run == models.DeliveryRunFailed {
				require.Equal(t, int(errs.CodeExecutionUnavailable), *store.outcomes[0].ErrorCode)
				require.Equal(t, errs.KeyExecutionUnavailable, *store.outcomes[0].ErrorKey)
			}
			require.Equal(t, uint64(3), inspector.application)
			require.Equal(t, "op", inspector.operation)
		})
	}
}

func TestReconcilePersistsUnknownAfterInspectionCancelsRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	store := &reconcileMemoryStore{candidate: &reconcileCandidate{Run: models.DeliveryRun{ID: 1}, Stage: models.DeliveryRunStage{ID: 2}}}
	inspector := &inspectionStub{err: context.Canceled, cancel: cancel}
	r := newReconciler(store, inspector, time.Now)
	require.NoError(t, r.Reconcile(ctx, 1))
	require.NoError(t, store.resolveContextErr)
	require.Equal(t, models.DeliveryRunOutcomeUnknown, store.outcomes[0].RunState)
	require.Zero(t, store.outcomes[0].Revision)
	require.Empty(t, store.outcomes[0].Digest)
}

func TestReconcileGenerationRequiresExactNonzeroMatch(t *testing.T) {
	require.True(t, sameReconcileGeneration(7, 7))
	require.False(t, sameReconcileGeneration(0, 0))
	require.False(t, sameReconcileGeneration(7, 8))
}

func TestReconcileLaterDefiniteEvidenceCanResolveUnknownRetry(t *testing.T) {
	target := "sha256:" + strings.Repeat("b", 64)
	store := &reconcileMemoryStore{candidate: &reconcileCandidate{Run: models.DeliveryRun{ID: 1, ChartDigest: target}, Stage: models.DeliveryRunStage{ID: 2}}}
	inspector := &inspectionStub{err: errors.New("ambiguous")}
	r := newReconciler(store, inspector, time.Now)
	require.NoError(t, r.Reconcile(context.Background(), 1))
	require.Equal(t, models.DeliveryRunOutcomeUnknown, store.outcomes[0].RunState)
	inspector.err = nil
	inspector.evidence = Inspection{Revision: 3, Digest: target}
	require.NoError(t, r.Reconcile(context.Background(), 1))
	require.Equal(t, models.DeliveryRunSucceeded, store.outcomes[1].RunState)
}
