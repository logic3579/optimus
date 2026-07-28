package run

import (
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/models"
)

func TestRunTransitionsMatchApprovedStateMachine(t *testing.T) {
	states := []models.DeliveryRunState{
		models.DeliveryRunQueued,
		models.DeliveryRunRunning,
		models.DeliveryRunWaitingApproval,
		models.DeliveryRunCancelRequested,
		models.DeliveryRunReconciling,
		models.DeliveryRunSucceeded,
		models.DeliveryRunFailed,
		models.DeliveryRunRejected,
		models.DeliveryRunCanceled,
		models.DeliveryRunTimedOut,
		models.DeliveryRunOutcomeUnknown,
	}
	legal := map[[2]models.DeliveryRunState]bool{
		{models.DeliveryRunQueued, models.DeliveryRunRunning}:              true,
		{models.DeliveryRunQueued, models.DeliveryRunCanceled}:             true,
		{models.DeliveryRunWaitingApproval, models.DeliveryRunQueued}:      true,
		{models.DeliveryRunWaitingApproval, models.DeliveryRunRejected}:    true,
		{models.DeliveryRunWaitingApproval, models.DeliveryRunCanceled}:    true,
		{models.DeliveryRunRunning, models.DeliveryRunWaitingApproval}:     true,
		{models.DeliveryRunRunning, models.DeliveryRunSucceeded}:           true,
		{models.DeliveryRunRunning, models.DeliveryRunFailed}:              true,
		{models.DeliveryRunRunning, models.DeliveryRunCancelRequested}:     true,
		{models.DeliveryRunRunning, models.DeliveryRunReconciling}:         true,
		{models.DeliveryRunCancelRequested, models.DeliveryRunCanceled}:    true,
		{models.DeliveryRunCancelRequested, models.DeliveryRunReconciling}: true,
		{models.DeliveryRunReconciling, models.DeliveryRunSucceeded}:       true,
		{models.DeliveryRunReconciling, models.DeliveryRunFailed}:          true,
		{models.DeliveryRunReconciling, models.DeliveryRunCanceled}:        true,
		{models.DeliveryRunReconciling, models.DeliveryRunRunning}:         true,
		{models.DeliveryRunReconciling, models.DeliveryRunWaitingApproval}: true,
		{models.DeliveryRunReconciling, models.DeliveryRunTimedOut}:        true,
		{models.DeliveryRunReconciling, models.DeliveryRunOutcomeUnknown}:  true,
		{models.DeliveryRunOutcomeUnknown, models.DeliveryRunReconciling}:  true,
	}

	for _, from := range states {
		for _, to := range states {
			require.Equalf(t, legal[[2]models.DeliveryRunState{from, to}], CanTransitionRun(from, to), "%s -> %s", from, to)
		}
	}
	require.False(t, CanTransitionRun("invalid", models.DeliveryRunQueued))
	require.False(t, CanTransitionRun(models.DeliveryRunQueued, "invalid"))
}

func TestStageTransitionsMatchApprovedStateMachine(t *testing.T) {
	states := []models.DeliveryStageState{
		models.DeliveryStagePending,
		models.DeliveryStageWaitingApproval,
		models.DeliveryStageQueued,
		models.DeliveryStageRunning,
		models.DeliveryStageReconciling,
		models.DeliveryStageSucceeded,
		models.DeliveryStageFailed,
		models.DeliveryStageRejected,
		models.DeliveryStageCanceled,
		models.DeliveryStageTimedOut,
		models.DeliveryStageOutcomeUnknown,
	}
	legal := map[[2]models.DeliveryStageState]bool{
		{models.DeliveryStagePending, models.DeliveryStageWaitingApproval}:    true,
		{models.DeliveryStagePending, models.DeliveryStageQueued}:             true,
		{models.DeliveryStagePending, models.DeliveryStageCanceled}:           true,
		{models.DeliveryStageWaitingApproval, models.DeliveryStageQueued}:     true,
		{models.DeliveryStageWaitingApproval, models.DeliveryStageRejected}:   true,
		{models.DeliveryStageWaitingApproval, models.DeliveryStageCanceled}:   true,
		{models.DeliveryStageQueued, models.DeliveryStageRunning}:             true,
		{models.DeliveryStageQueued, models.DeliveryStageCanceled}:            true,
		{models.DeliveryStageRunning, models.DeliveryStageSucceeded}:          true,
		{models.DeliveryStageRunning, models.DeliveryStageFailed}:             true,
		{models.DeliveryStageRunning, models.DeliveryStageReconciling}:        true,
		{models.DeliveryStageReconciling, models.DeliveryStageSucceeded}:      true,
		{models.DeliveryStageReconciling, models.DeliveryStageFailed}:         true,
		{models.DeliveryStageReconciling, models.DeliveryStageCanceled}:       true,
		{models.DeliveryStageReconciling, models.DeliveryStageTimedOut}:       true,
		{models.DeliveryStageReconciling, models.DeliveryStageOutcomeUnknown}: true,
		{models.DeliveryStageOutcomeUnknown, models.DeliveryStageReconciling}: true,
	}

	for _, from := range states {
		for _, to := range states {
			require.Equalf(t, legal[[2]models.DeliveryStageState{from, to}], CanTransitionStage(from, to), "%s -> %s", from, to)
		}
	}
	require.False(t, CanTransitionStage("invalid", models.DeliveryStagePending))
	require.False(t, CanTransitionStage(models.DeliveryStagePending, "invalid"))
}
