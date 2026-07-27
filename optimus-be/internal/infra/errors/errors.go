package errors

import (
	"errors"
	"fmt"
)

type BizError struct {
	Code       Code
	MessageKey string
	Message    string
	Cause      error
}

func New(code Code, messageKey, message string) *BizError {
	return &BizError{Code: code, MessageKey: messageKey, Message: message}
}

func Wrap(cause error, code Code, messageKey, message string) *BizError {
	return &BizError{Code: code, MessageKey: messageKey, Message: message, Cause: cause}
}

func (e *BizError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *BizError) Unwrap() error { return e.Cause }

// AsBiz pulls a BizError out of a wrapped error chain.
func AsBiz(err error) (*BizError, bool) {
	var be *BizError
	if errors.As(err, &be) {
		return be, true
	}
	return nil, false
}

// HTTPStatus maps a business Code to an HTTP status code.
func HTTPStatus(c Code) int {
	switch {
	case c == CodeOK:
		return 200
	case c >= 10000 && c < 20000:
		return 500
	case c == CodeRateLimited:
		return 429
	case c == CodeDeliveryProjectNotFound ||
		c == CodeDeliveryPipelineMissing ||
		c == CodeDeliveryEnvironmentNotFound ||
		c == CodeDeliveryRunNotFound ||
		c == CodeDeliveryApprovalNotFound:
		return 404
	case c == CodeDeliveryProjectNameConflict ||
		c == CodeDeliveryApplicationAlreadyBound ||
		c == CodeDeliveryPipelineVersionConflict ||
		c == CodeDeliveryEnvironmentInUse ||
		c == CodeDeliveryActiveRun ||
		c == CodeDeliveryIdempotencyConflict ||
		c == CodeDeliveryRunInvalidState ||
		c == CodeDeliveryRunCancelConflict ||
		c == CodeDeliveryRunRetryUnavailable ||
		c == CodeDeliveryApprovalAlreadyDecided ||
		c == CodeDeliveryApprovalDecisionConflict ||
		c == CodeDeliveryOperationBusy ||
		c == CodeDeliveryArtifactDrift ||
		c == CodeDeliveryReconciliationRequired ||
		c == CodeDeliveryOutcomeUnknown:
		return 409
	case c == CodeDeliveryApprovalSelfApproval:
		return 403
	case c == CodeDeliveryExecutionTimeout:
		return 504
	case c == CodeDeliveryExecutionUnavailable:
		return 503
	case c == CodeDeliveryApplicationUnavailable ||
		c == CodeDeliveryChartIdentityMismatch ||
		c == CodeDeliveryPipelineInvalid:
		return 422
	case c == CodeDeliveryIdempotencyMissing ||
		c == CodeDeliveryApprovalCommentRequired ||
		c == CodeDeliveryApprovalCommentInvalid:
		return 400
	case c == CodeAssetsCloudAccountInUse || c == CodeAssetsSyncBusy:
		return 409
	case c == CodeObservabilityDatasourceNotFound ||
		c == CodeObservabilityDashboardNotFound ||
		c == CodeObservabilityDashboardBuiltinNotFound:
		return 404
	case c == CodeObservabilityDatasourceNameTaken ||
		c == CodeObservabilityDatasourceInUse ||
		c == CodeObservabilityDashboardNameTaken:
		return 409
	case c == CodeObservabilityQueryDestinationDenied:
		return 403
	case c == CodeObservabilityQueryUpstreamTimeout:
		return 504
	case c == CodeObservabilityQueryUpstreamUnreachable ||
		c == CodeObservabilityQueryUpstreamRejected ||
		c == CodeObservabilityQueryInvalidResponse:
		return 502
	case c == CodeObservabilityDatasourceInvalidURL ||
		c == CodeObservabilityDatasourceAuthMismatch ||
		c == CodeObservabilityDatasourceInvalidTLS ||
		c == CodeObservabilityQueryLimitExceeded ||
		c == CodeObservabilityQueryInvalidRequest ||
		c == CodeObservabilityDashboardInvalidPanel:
		return 400
	case c == CodeAssetsCloudAccountNotFound || c == CodeAssetsVPCNotFound:
		return 404
	case c == CodeAssetsCloudAccountNameConflict ||
		c == CodeAssetsRegionInvalid ||
		c == CodeAssetsProviderUnsupported ||
		c == CodeAssetsCloudAccountDisabled ||
		c == CodeAssetsCloudKeyNotFound:
		return 422
	case c >= 40400 && c < 40500:
		return 404
	case c >= 40300 && c < 40400:
		return 403
	case c >= 40100 && c < 40200:
		return 401
	case c >= 40900 && c < 41000:
		return 409
	case c >= 40000 && c < 41000:
		return 400
	case c >= 50000 && c < 60000:
		return 500
	default:
		return 500
	}
}
