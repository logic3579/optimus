package account

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/modules/assets/errs"
)

func TestMapWriteError_NameConstraintOnly(t *testing.T) {
	mapped := mapWriteError(&pgconn.PgError{Code: "23505", ConstraintName: "uq_cloud_accounts_name_alive"})
	var bizErr *apperr.BizError
	require.True(t, errors.As(mapped, &bizErr))
	require.Equal(t, errs.CodeAssetsCloudAccountNameConflict, bizErr.Code)

	other := &pgconn.PgError{Code: "23505", ConstraintName: "other_constraint"}
	require.ErrorIs(t, mapWriteError(other), other)
	sentinel := errors.New("db down")
	require.ErrorIs(t, mapWriteError(sentinel), sentinel)
}
