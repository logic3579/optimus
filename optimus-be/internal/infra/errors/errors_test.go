package errors_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	apperr "optimus-be/internal/infra/errors"
)

func TestNew_HasCodeAndMessageKey(t *testing.T) {
	e := apperr.New(apperr.CodeInvalidCredentials, "auth.invalid_credentials", "invalid username or password")
	require.Equal(t, apperr.CodeInvalidCredentials, e.Code)
	require.Equal(t, "auth.invalid_credentials", e.MessageKey)
	require.Equal(t, "invalid username or password", e.Error())
}

func TestWrap_PreservesCause(t *testing.T) {
	cause := errors.New("db dead")
	e := apperr.Wrap(cause, apperr.CodeDBError, "db.error", "database failure")
	require.ErrorIs(t, e, cause)
	require.Equal(t, apperr.CodeDBError, e.Code)
}

func TestAsBizError(t *testing.T) {
	e := apperr.New(apperr.CodeNotFound, "common.not_found", "not found")
	var be *apperr.BizError
	require.True(t, errors.As(e, &be))
	require.Equal(t, apperr.CodeNotFound, be.Code)
}

func TestHTTPStatus_DerivedFromCode(t *testing.T) {
	require.Equal(t, 400, apperr.HTTPStatus(apperr.CodeBadRequest))
	require.Equal(t, 401, apperr.HTTPStatus(apperr.CodeInvalidCredentials))
	require.Equal(t, 403, apperr.HTTPStatus(apperr.CodeForbidden))
	require.Equal(t, 404, apperr.HTTPStatus(apperr.CodeNotFound))
	require.Equal(t, 409, apperr.HTTPStatus(apperr.CodeConflict))
	require.Equal(t, 429, apperr.HTTPStatus(apperr.CodeRateLimited))
	require.Equal(t, 500, apperr.HTTPStatus(apperr.CodeInternal))
}

func TestHTTPStatus_AssetsDomainCodes(t *testing.T) {
	tests := []struct {
		name string
		code apperr.Code
		want int
	}{
		{"cloud account in use", apperr.CodeAssetsCloudAccountInUse, 409},
		{"cloud account not found", apperr.CodeAssetsCloudAccountNotFound, 404},
		{"cloud account name conflict", apperr.CodeAssetsCloudAccountNameConflict, 422},
		{"invalid region", apperr.CodeAssetsRegionInvalid, 422},
		{"unsupported provider", apperr.CodeAssetsProviderUnsupported, 422},
		{"disabled account", apperr.CodeAssetsCloudAccountDisabled, 422},
		{"cloud key not found", apperr.CodeAssetsCloudKeyNotFound, 422},
		{"VPC not found", apperr.CodeAssetsVPCNotFound, 404},
		{"sync busy", apperr.CodeAssetsSyncBusy, 409},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, apperr.HTTPStatus(tt.code))
		})
	}
}

func TestHTTPStatus_ObservabilityDomainCodes(t *testing.T) {
	tests := []struct {
		name string
		code apperr.Code
		want int
	}{
		{"data source not found", apperr.CodeObservabilityDatasourceNotFound, 404},
		{"data source name taken", apperr.CodeObservabilityDatasourceNameTaken, 409},
		{"data source in use", apperr.CodeObservabilityDatasourceInUse, 409},
		{"invalid data source URL", apperr.CodeObservabilityDatasourceInvalidURL, 400},
		{"data source auth mismatch", apperr.CodeObservabilityDatasourceAuthMismatch, 400},
		{"invalid data source TLS", apperr.CodeObservabilityDatasourceInvalidTLS, 400},
		{"query destination denied", apperr.CodeObservabilityQueryDestinationDenied, 403},
		{"query upstream unreachable", apperr.CodeObservabilityQueryUpstreamUnreachable, 502},
		{"query upstream timeout", apperr.CodeObservabilityQueryUpstreamTimeout, 504},
		{"query upstream rejected", apperr.CodeObservabilityQueryUpstreamRejected, 502},
		{"query invalid response", apperr.CodeObservabilityQueryInvalidResponse, 502},
		{"query limit exceeded", apperr.CodeObservabilityQueryLimitExceeded, 400},
		{"query invalid request", apperr.CodeObservabilityQueryInvalidRequest, 400},
		{"dashboard not found", apperr.CodeObservabilityDashboardNotFound, 404},
		{"dashboard name taken", apperr.CodeObservabilityDashboardNameTaken, 409},
		{"dashboard invalid panel", apperr.CodeObservabilityDashboardInvalidPanel, 400},
		{"dashboard builtin not found", apperr.CodeObservabilityDashboardBuiltinNotFound, 404},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, apperr.HTTPStatus(tt.code))
		})
	}
}

func TestHTTPStatus_DeliveryDomainCodes(t *testing.T) {
	tests := []struct {
		name string
		code apperr.Code
		want int
	}{
		{"project not found", apperr.CodeDeliveryProjectNotFound, 404},
		{"project name conflict", apperr.CodeDeliveryProjectNameConflict, 409},
		{"application already bound", apperr.CodeDeliveryApplicationAlreadyBound, 409},
		{"application unavailable", apperr.CodeDeliveryApplicationUnavailable, 422},
		{"chart identity mismatch", apperr.CodeDeliveryChartIdentityMismatch, 422},
		{"pipeline missing", apperr.CodeDeliveryPipelineMissing, 404},
		{"pipeline invalid", apperr.CodeDeliveryPipelineInvalid, 422},
		{"pipeline version conflict", apperr.CodeDeliveryPipelineVersionConflict, 409},
		{"environment not found", apperr.CodeDeliveryEnvironmentNotFound, 404},
		{"environment in use", apperr.CodeDeliveryEnvironmentInUse, 409},
		{"run not found", apperr.CodeDeliveryRunNotFound, 404},
		{"active run", apperr.CodeDeliveryActiveRun, 409},
		{"idempotency conflict", apperr.CodeDeliveryIdempotencyConflict, 409},
		{"idempotency missing", apperr.CodeDeliveryIdempotencyMissing, 400},
		{"run invalid state", apperr.CodeDeliveryRunInvalidState, 409},
		{"run cancel conflict", apperr.CodeDeliveryRunCancelConflict, 409},
		{"run retry unavailable", apperr.CodeDeliveryRunRetryUnavailable, 409},
		{"approval not found", apperr.CodeDeliveryApprovalNotFound, 404},
		{"self approval", apperr.CodeDeliveryApprovalSelfApproval, 403},
		{"approval already decided", apperr.CodeDeliveryApprovalAlreadyDecided, 409},
		{"approval decision conflict", apperr.CodeDeliveryApprovalDecisionConflict, 409},
		{"approval comment required", apperr.CodeDeliveryApprovalCommentRequired, 400},
		{"approval comment invalid", apperr.CodeDeliveryApprovalCommentInvalid, 400},
		{"operation busy", apperr.CodeDeliveryOperationBusy, 409},
		{"artifact drift", apperr.CodeDeliveryArtifactDrift, 409},
		{"reconciliation required", apperr.CodeDeliveryReconciliationRequired, 409},
		{"outcome unknown", apperr.CodeDeliveryOutcomeUnknown, 409},
		{"execution timeout", apperr.CodeDeliveryExecutionTimeout, 504},
		{"execution unavailable", apperr.CodeDeliveryExecutionUnavailable, 503},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, apperr.HTTPStatus(tt.code))
		})
	}
}
