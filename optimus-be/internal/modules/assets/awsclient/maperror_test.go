package awsclient

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/require"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/modules/assets/errs"
)

type timeoutError struct{}

func (timeoutError) Error() string   { return "SECRET timeout detail" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func TestMapError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code apperr.Code
		key  string
	}{
		{"nil", nil, 0, ""},
		{"deadline", fmt.Errorf("wrapped: %w", context.DeadlineExceeded), errs.CodeAssetsAWSUnreachable, errs.KeyAWSUnreachable},
		{"canceled", fmt.Errorf("wrapped: %w", context.Canceled), errs.CodeAssetsAWSUnreachable, errs.KeyAWSUnreachable},
		{"net error", timeoutError{}, errs.CodeAssetsAWSUnreachable, errs.KeyAWSUnreachable},
		{"SDK request canceled", fmt.Errorf("wrapped: %w", &aws.RequestCanceledError{Err: errors.New("SECRET canceled")}), errs.CodeAssetsAWSUnreachable, errs.KeyAWSUnreachable},
		{"Smithy request canceled", apiError("RequestCanceled"), errs.CodeAssetsAWSUnreachable, errs.KeyAWSUnreachable},
		{"auth failure", apiError("AuthFailure"), errs.CodeAssetsAWSUnauthorized, errs.KeyAWSUnauthorized},
		{"invalid token", apiError("InvalidClientTokenId"), errs.CodeAssetsAWSUnauthorized, errs.KeyAWSUnauthorized},
		{"bad signature", apiError("SignatureDoesNotMatch"), errs.CodeAssetsAWSUnauthorized, errs.KeyAWSUnauthorized},
		{"expired token", apiError("ExpiredToken"), errs.CodeAssetsAWSUnauthorized, errs.KeyAWSUnauthorized},
		{"unauthorized operation", apiError("UnauthorizedOperation"), errs.CodeAssetsAWSForbidden, errs.KeyAWSForbidden},
		{"access denied", apiError("AccessDenied"), errs.CodeAssetsAWSForbidden, errs.KeyAWSForbidden},
		{"access denied exception", apiError("AccessDeniedException"), errs.CodeAssetsAWSForbidden, errs.KeyAWSForbidden},
		{"throttling", apiError("Throttling"), errs.CodeAssetsAWSThrottled, errs.KeyAWSThrottled},
		{"throttling exception", apiError("ThrottlingException"), errs.CodeAssetsAWSThrottled, errs.KeyAWSThrottled},
		{"request limit", apiError("RequestLimitExceeded"), errs.CodeAssetsAWSThrottled, errs.KeyAWSThrottled},
		{"too many requests", apiError("TooManyRequestsException"), errs.CodeAssetsAWSThrottled, errs.KeyAWSThrottled},
		{"unknown Smithy", apiError("SomethingElse"), errs.CodeAssetsAWSOther, errs.KeyAWSOther},
		{"plain", errors.New("SECRET raw failure"), errs.CodeAssetsAWSOther, errs.KeyAWSOther},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, key, message := MapError(tt.err)
			require.Equal(t, tt.code, code)
			require.Equal(t, tt.key, key)
			if tt.err == nil {
				require.Empty(t, message)
				return
			}
			require.NotEmpty(t, message)
			require.NotContains(t, strings.ToUpper(message), "SECRET")
		})
	}
}

func apiError(code string) error {
	return fmt.Errorf("outer: %w", &smithy.GenericAPIError{Code: code, Message: "SECRET upstream detail"})
}
