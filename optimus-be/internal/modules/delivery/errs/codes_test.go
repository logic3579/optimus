package errs

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodes_AliasEveryDeliveryError(t *testing.T) {
	codes := []int{
		int(CodeProjectNotFound), int(CodeProjectNameConflict), int(CodeApplicationAlreadyBound), int(CodeApplicationUnavailable), int(CodeChartIdentityMismatch),
		int(CodePipelineMissing), int(CodePipelineInvalid), int(CodePipelineVersionConflict), int(CodeEnvironmentNotFound), int(CodeEnvironmentInUse),
		int(CodeRunNotFound), int(CodeActiveRun), int(CodeIdempotencyConflict), int(CodeIdempotencyMissing), int(CodeRunInvalidState), int(CodeRunCancelConflict), int(CodeRunRetryUnavailable),
		int(CodeApprovalNotFound), int(CodeApprovalSelfApproval), int(CodeApprovalAlreadyDecided), int(CodeApprovalDecisionConflict), int(CodeApprovalCommentRequired), int(CodeApprovalCommentInvalid),
		int(CodeOperationBusy), int(CodeArtifactDrift), int(CodeReconciliationRequired), int(CodeOutcomeUnknown), int(CodeExecutionTimeout), int(CodeExecutionUnavailable),
	}
	require.Equal(t, 45001, int(CodeProjectNotFound))
	require.Equal(t, 45101, int(CodeRunNotFound))
	require.Equal(t, 45201, int(CodeApprovalNotFound))
	require.Equal(t, 45301, int(CodeOperationBusy))

	seen := make(map[int]struct{}, len(codes))
	for _, code := range codes {
		_, duplicate := seen[code]
		require.Falsef(t, duplicate, "duplicate delivery error code %d", code)
		seen[code] = struct{}{}
	}
	require.Len(t, seen, len(codes))
}

func TestKeys_BeginWithDelivery(t *testing.T) {
	keys := []string{
		KeyProjectNotFound, KeyProjectNameConflict, KeyApplicationAlreadyBound, KeyApplicationUnavailable, KeyChartIdentityMismatch,
		KeyPipelineMissing, KeyPipelineInvalid, KeyPipelineVersionConflict, KeyEnvironmentNotFound, KeyEnvironmentInUse,
		KeyRunNotFound, KeyActiveRun, KeyIdempotencyConflict, KeyIdempotencyMissing, KeyRunInvalidState, KeyRunCancelConflict, KeyRunRetryUnavailable,
		KeyApprovalNotFound, KeyApprovalSelfApproval, KeyApprovalAlreadyDecided, KeyApprovalDecisionConflict, KeyApprovalCommentRequired, KeyApprovalCommentInvalid,
		KeyOperationBusy, KeyArtifactDrift, KeyReconciliationRequired, KeyOutcomeUnknown, KeyExecutionTimeout, KeyExecutionUnavailable,
	}
	for _, key := range keys {
		require.Truef(t, strings.HasPrefix(key, "delivery."), "message key %q must begin with delivery.", key)
	}
}
