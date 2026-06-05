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
