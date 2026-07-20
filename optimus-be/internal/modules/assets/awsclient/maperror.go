package awsclient

import (
	"context"
	"errors"
	"net"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/smithy-go"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/modules/assets/errs"
)

// MapError normalizes AWS failures for sync-run persistence. Returned messages
// are stable and safe for clients; upstream error text is intentionally omitted.
func MapError(err error) (apperr.Code, string, string) {
	if err == nil {
		return 0, "", ""
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return unreachableError()
	}
	var canceled *aws.RequestCanceledError
	if errors.As(err, &canceled) {
		return unreachableError()
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		return unreachableError()
	}
	var apiError smithy.APIError
	if errors.As(err, &apiError) {
		switch apiError.ErrorCode() {
		case "AuthFailure", "InvalidClientTokenId", "SignatureDoesNotMatch", "ExpiredToken":
			return errs.CodeAssetsAWSUnauthorized, errs.KeyAWSUnauthorized, "AWS credentials invalid or expired"
		case "UnauthorizedOperation", "AccessDenied", "AccessDeniedException":
			return errs.CodeAssetsAWSForbidden, errs.KeyAWSForbidden, "AWS API access denied"
		case "RequestCanceled":
			return unreachableError()
		case "Throttling", "ThrottlingException", "RequestLimitExceeded", "TooManyRequestsException":
			return errs.CodeAssetsAWSThrottled, errs.KeyAWSThrottled, "AWS API request throttled"
		}
	}
	return errs.CodeAssetsAWSOther, errs.KeyAWSOther, "AWS API request failed"
}

func unreachableError() (apperr.Code, string, string) {
	return errs.CodeAssetsAWSUnreachable, errs.KeyAWSUnreachable, "AWS API unreachable"
}
