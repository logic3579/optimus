package run

import "optimus-be/internal/models"

var runTransitions = map[models.DeliveryRunState]map[models.DeliveryRunState]struct{}{
	models.DeliveryRunQueued: {
		models.DeliveryRunRunning: {}, models.DeliveryRunCanceled: {},
	},
	models.DeliveryRunWaitingApproval: {
		models.DeliveryRunQueued: {}, models.DeliveryRunRejected: {}, models.DeliveryRunCanceled: {},
	},
	models.DeliveryRunRunning: {
		models.DeliveryRunWaitingApproval: {}, models.DeliveryRunSucceeded: {}, models.DeliveryRunFailed: {},
		models.DeliveryRunCancelRequested: {}, models.DeliveryRunReconciling: {},
	},
	models.DeliveryRunCancelRequested: {
		models.DeliveryRunCanceled: {}, models.DeliveryRunReconciling: {},
	},
	models.DeliveryRunReconciling: {
		models.DeliveryRunSucceeded: {}, models.DeliveryRunFailed: {}, models.DeliveryRunCanceled: {},
		models.DeliveryRunTimedOut: {}, models.DeliveryRunOutcomeUnknown: {},
	},
	models.DeliveryRunOutcomeUnknown: {
		models.DeliveryRunReconciling: {},
	},
}

var stageTransitions = map[models.DeliveryStageState]map[models.DeliveryStageState]struct{}{
	models.DeliveryStagePending: {
		models.DeliveryStageWaitingApproval: {}, models.DeliveryStageQueued: {}, models.DeliveryStageCanceled: {},
	},
	models.DeliveryStageWaitingApproval: {
		models.DeliveryStageQueued: {}, models.DeliveryStageRejected: {}, models.DeliveryStageCanceled: {},
	},
	models.DeliveryStageQueued: {
		models.DeliveryStageRunning: {}, models.DeliveryStageCanceled: {},
	},
	models.DeliveryStageRunning: {
		models.DeliveryStageSucceeded: {}, models.DeliveryStageFailed: {}, models.DeliveryStageReconciling: {},
	},
	models.DeliveryStageReconciling: {
		models.DeliveryStageSucceeded: {}, models.DeliveryStageFailed: {}, models.DeliveryStageCanceled: {},
		models.DeliveryStageTimedOut: {}, models.DeliveryStageOutcomeUnknown: {},
	},
	models.DeliveryStageOutcomeUnknown: {
		models.DeliveryStageReconciling: {},
	},
}

// CanTransitionRun reports whether the approved P6 run state machine permits
// an expected-old-state update from from to to.
func CanTransitionRun(from, to models.DeliveryRunState) bool {
	_, ok := runTransitions[from][to]
	return ok
}

// CanTransitionStage reports whether the approved P6 stage state machine
// permits an expected-old-state update from from to to.
func CanTransitionStage(from, to models.DeliveryStageState) bool {
	_, ok := stageTransitions[from][to]
	return ok
}
