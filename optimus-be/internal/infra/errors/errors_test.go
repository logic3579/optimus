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
