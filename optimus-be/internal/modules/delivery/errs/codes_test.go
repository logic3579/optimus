package errs

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodes_ExactAliasesAndKeys(t *testing.T) {
	tests := []struct {
		name    string
		code    int
		want    int
		key     string
		wantKey string
	}{
		{"CodeProjectNotFound", int(CodeProjectNotFound), 45001, KeyProjectNotFound, "delivery.project.not_found"},
		{"CodeProjectNameConflict", int(CodeProjectNameConflict), 45002, KeyProjectNameConflict, "delivery.project.name_conflict"},
		{"CodeApplicationAlreadyBound", int(CodeApplicationAlreadyBound), 45003, KeyApplicationAlreadyBound, "delivery.application.already_bound"},
		{"CodeApplicationUnavailable", int(CodeApplicationUnavailable), 45004, KeyApplicationUnavailable, "delivery.application.unavailable"},
		{"CodeChartIdentityMismatch", int(CodeChartIdentityMismatch), 45005, KeyChartIdentityMismatch, "delivery.chart.identity_mismatch"},
		{"CodePipelineMissing", int(CodePipelineMissing), 45006, KeyPipelineMissing, "delivery.pipeline.missing"},
		{"CodePipelineInvalid", int(CodePipelineInvalid), 45007, KeyPipelineInvalid, "delivery.pipeline.invalid"},
		{"CodePipelineVersionConflict", int(CodePipelineVersionConflict), 45008, KeyPipelineVersionConflict, "delivery.pipeline.version_conflict"},
		{"CodeEnvironmentNotFound", int(CodeEnvironmentNotFound), 45009, KeyEnvironmentNotFound, "delivery.environment.not_found"},
		{"CodeEnvironmentInUse", int(CodeEnvironmentInUse), 45010, KeyEnvironmentInUse, "delivery.environment.in_use"},
		{"CodeRunNotFound", int(CodeRunNotFound), 45101, KeyRunNotFound, "delivery.run.not_found"},
		{"CodeActiveRun", int(CodeActiveRun), 45102, KeyActiveRun, "delivery.run.active"},
		{"CodeIdempotencyConflict", int(CodeIdempotencyConflict), 45103, KeyIdempotencyConflict, "delivery.run.idempotency_conflict"},
		{"CodeIdempotencyMissing", int(CodeIdempotencyMissing), 45104, KeyIdempotencyMissing, "delivery.run.idempotency_missing"},
		{"CodeRunInvalidState", int(CodeRunInvalidState), 45105, KeyRunInvalidState, "delivery.run.invalid_state"},
		{"CodeRunCancelConflict", int(CodeRunCancelConflict), 45106, KeyRunCancelConflict, "delivery.run.cancel_conflict"},
		{"CodeRunRetryUnavailable", int(CodeRunRetryUnavailable), 45107, KeyRunRetryUnavailable, "delivery.run.retry_unavailable"},
		{"CodeApprovalNotFound", int(CodeApprovalNotFound), 45201, KeyApprovalNotFound, "delivery.approval.not_found"},
		{"CodeApprovalSelfApproval", int(CodeApprovalSelfApproval), 45202, KeyApprovalSelfApproval, "delivery.approval.self_approval"},
		{"CodeApprovalAlreadyDecided", int(CodeApprovalAlreadyDecided), 45203, KeyApprovalAlreadyDecided, "delivery.approval.already_decided"},
		{"CodeApprovalDecisionConflict", int(CodeApprovalDecisionConflict), 45204, KeyApprovalDecisionConflict, "delivery.approval.decision_conflict"},
		{"CodeApprovalCommentRequired", int(CodeApprovalCommentRequired), 45205, KeyApprovalCommentRequired, "delivery.approval.comment_required"},
		{"CodeApprovalCommentInvalid", int(CodeApprovalCommentInvalid), 45206, KeyApprovalCommentInvalid, "delivery.approval.comment_invalid"},
		{"CodeOperationBusy", int(CodeOperationBusy), 45301, KeyOperationBusy, "delivery.execution.operation_busy"},
		{"CodeArtifactDrift", int(CodeArtifactDrift), 45302, KeyArtifactDrift, "delivery.execution.artifact_drift"},
		{"CodeReconciliationRequired", int(CodeReconciliationRequired), 45303, KeyReconciliationRequired, "delivery.execution.reconciliation_required"},
		{"CodeOutcomeUnknown", int(CodeOutcomeUnknown), 45304, KeyOutcomeUnknown, "delivery.execution.outcome_unknown"},
		{"CodeExecutionTimeout", int(CodeExecutionTimeout), 45305, KeyExecutionTimeout, "delivery.execution.timeout"},
		{"CodeExecutionUnavailable", int(CodeExecutionUnavailable), 45306, KeyExecutionUnavailable, "delivery.execution.unavailable"},
	}

	require.Len(t, tests, 29)
	codes := make(map[int]string, len(tests))
	keys := make(map[string]string, len(tests))
	for _, tt := range tests {
		require.Equalf(t, tt.want, tt.code, "%s numeric alias", tt.name)
		require.Equalf(t, tt.wantKey, tt.key, "%s message key", tt.name)
		_, duplicateCode := codes[tt.code]
		require.Falsef(t, duplicateCode, "%s duplicates code %d", tt.name, tt.code)
		codes[tt.code] = tt.name
		_, duplicateKey := keys[tt.key]
		require.Falsef(t, duplicateKey, "%s duplicates key %q", tt.name, tt.key)
		keys[tt.key] = tt.name
		require.Truef(t, strings.HasPrefix(tt.key, "delivery."), "%s key %q must begin with delivery.", tt.name, tt.key)
	}
}
