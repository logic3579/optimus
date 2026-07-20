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
