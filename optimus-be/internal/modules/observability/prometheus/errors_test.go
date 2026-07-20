package prometheus

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	apperr "optimus-be/internal/infra/errors"
)

func TestMapClientErrorUsesStableMessageAndCause(t *testing.T) {
	cause := errors.New("sensitive detail")
	err := mapClientError(cause)
	be := requireClientBizCode(t, err, apperr.CodeObservabilityQueryUpstreamUnreachable)
	require.Equal(t, "observability query upstream unreachable", be.Message)
	require.ErrorIs(t, be, cause)
}

func TestExpressionRejectedIsDistinguishableFromHTTPRejected(t *testing.T) {
	expression := expressionRejected(errors.New("bad expression"))
	require.True(t, IsExpressionError(expression))
	be := requireClientBizCode(t, expression, apperr.CodeObservabilityQueryUpstreamRejected)
	require.Equal(t, "observability query upstream rejected", be.Message)

	httpFailure := rejected(errors.New("HTTP 401"))
	require.False(t, IsExpressionError(httpFailure))
	requireClientBizCode(t, httpFailure, apperr.CodeObservabilityQueryUpstreamRejected)
}
