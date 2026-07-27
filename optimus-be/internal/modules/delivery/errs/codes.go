package errs

import infraerrors "optimus-be/internal/infra/errors"

const (
	// Project, environment, and pipeline errors.
	CodeProjectNotFound         = infraerrors.CodeDeliveryProjectNotFound
	CodeProjectNameConflict     = infraerrors.CodeDeliveryProjectNameConflict
	CodeApplicationAlreadyBound = infraerrors.CodeDeliveryApplicationAlreadyBound
	CodeApplicationUnavailable  = infraerrors.CodeDeliveryApplicationUnavailable
	CodeChartIdentityMismatch   = infraerrors.CodeDeliveryChartIdentityMismatch
	CodePipelineMissing         = infraerrors.CodeDeliveryPipelineMissing
	CodePipelineInvalid         = infraerrors.CodeDeliveryPipelineInvalid
	CodePipelineVersionConflict = infraerrors.CodeDeliveryPipelineVersionConflict
	CodeEnvironmentNotFound     = infraerrors.CodeDeliveryEnvironmentNotFound
	CodeEnvironmentInUse        = infraerrors.CodeDeliveryEnvironmentInUse

	// Run errors.
	CodeRunNotFound         = infraerrors.CodeDeliveryRunNotFound
	CodeActiveRun           = infraerrors.CodeDeliveryActiveRun
	CodeIdempotencyConflict = infraerrors.CodeDeliveryIdempotencyConflict
	CodeIdempotencyMissing  = infraerrors.CodeDeliveryIdempotencyMissing
	CodeRunInvalidState     = infraerrors.CodeDeliveryRunInvalidState
	CodeRunCancelConflict   = infraerrors.CodeDeliveryRunCancelConflict
	CodeRunRetryUnavailable = infraerrors.CodeDeliveryRunRetryUnavailable

	// Approval errors.
	CodeApprovalNotFound         = infraerrors.CodeDeliveryApprovalNotFound
	CodeApprovalSelfApproval     = infraerrors.CodeDeliveryApprovalSelfApproval
	CodeApprovalAlreadyDecided   = infraerrors.CodeDeliveryApprovalAlreadyDecided
	CodeApprovalDecisionConflict = infraerrors.CodeDeliveryApprovalDecisionConflict
	CodeApprovalCommentRequired  = infraerrors.CodeDeliveryApprovalCommentRequired
	CodeApprovalCommentInvalid   = infraerrors.CodeDeliveryApprovalCommentInvalid

	// Execution errors.
	CodeOperationBusy          = infraerrors.CodeDeliveryOperationBusy
	CodeArtifactDrift          = infraerrors.CodeDeliveryArtifactDrift
	CodeReconciliationRequired = infraerrors.CodeDeliveryReconciliationRequired
	CodeOutcomeUnknown         = infraerrors.CodeDeliveryOutcomeUnknown
	CodeExecutionTimeout       = infraerrors.CodeDeliveryExecutionTimeout
	CodeExecutionUnavailable   = infraerrors.CodeDeliveryExecutionUnavailable
)

const (
	// Project, environment, and pipeline message keys.
	KeyProjectNotFound         = "delivery.project.not_found"
	KeyProjectNameConflict     = "delivery.project.name_conflict"
	KeyApplicationAlreadyBound = "delivery.application.already_bound"
	KeyApplicationUnavailable  = "delivery.application.unavailable"
	KeyChartIdentityMismatch   = "delivery.chart.identity_mismatch"
	KeyPipelineMissing         = "delivery.pipeline.missing"
	KeyPipelineInvalid         = "delivery.pipeline.invalid"
	KeyPipelineVersionConflict = "delivery.pipeline.version_conflict"
	KeyEnvironmentNotFound     = "delivery.environment.not_found"
	KeyEnvironmentInUse        = "delivery.environment.in_use"

	// Run message keys.
	KeyRunNotFound         = "delivery.run.not_found"
	KeyActiveRun           = "delivery.run.active"
	KeyIdempotencyConflict = "delivery.run.idempotency_conflict"
	KeyIdempotencyMissing  = "delivery.run.idempotency_missing"
	KeyRunInvalidState     = "delivery.run.invalid_state"
	KeyRunCancelConflict   = "delivery.run.cancel_conflict"
	KeyRunRetryUnavailable = "delivery.run.retry_unavailable"

	// Approval message keys.
	KeyApprovalNotFound         = "delivery.approval.not_found"
	KeyApprovalSelfApproval     = "delivery.approval.self_approval"
	KeyApprovalAlreadyDecided   = "delivery.approval.already_decided"
	KeyApprovalDecisionConflict = "delivery.approval.decision_conflict"
	KeyApprovalCommentRequired  = "delivery.approval.comment_required"
	KeyApprovalCommentInvalid   = "delivery.approval.comment_invalid"

	// Execution message keys.
	KeyOperationBusy          = "delivery.execution.operation_busy"
	KeyArtifactDrift          = "delivery.execution.artifact_drift"
	KeyReconciliationRequired = "delivery.execution.reconciliation_required"
	KeyOutcomeUnknown         = "delivery.execution.outcome_unknown"
	KeyExecutionTimeout       = "delivery.execution.timeout"
	KeyExecutionUnavailable   = "delivery.execution.unavailable"
)
