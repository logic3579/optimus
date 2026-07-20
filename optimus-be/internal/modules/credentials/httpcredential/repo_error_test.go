package httpcredential

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	apperr "optimus-be/internal/infra/errors"
)

func TestMapWriteErrorMapsOnlyHTTPNameUniqueViolation(t *testing.T) {
	err := mapWriteError(&pgconn.PgError{Code: "23505", ConstraintName: "credentials_http_name_unique"})
	b, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeConflict, b.Code)
	require.Equal(t, "credentials.name_taken", b.MessageKey)
	other := &pgconn.PgError{Code: "23505", ConstraintName: "other"}
	require.Same(t, other, mapWriteError(other))
	plain := errors.New("db unavailable")
	require.ErrorIs(t, mapWriteError(plain), plain)
}
