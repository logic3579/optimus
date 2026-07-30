package project

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/modules/delivery/errs"
)

func TestRepositoryErrorMappings(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		mapper     func(error) error
		wantedCode apperr.Code
	}{
		{name: "project duplicate", err: &pgconn.PgError{Code: "23505", ConstraintName: "delivery_projects_active_name_unique"}, mapper: mapProjectWriteError, wantedCode: errs.CodeProjectNameConflict},
		{name: "application duplicate", err: &pgconn.PgError{Code: "23505", ConstraintName: "delivery_environments_active_application_unique"}, mapper: mapEnvironmentWriteError, wantedCode: errs.CodeApplicationAlreadyBound},
		{name: "environment key duplicate", err: &pgconn.PgError{Code: "23505", ConstraintName: "delivery_environments_active_project_key_unique"}, mapper: mapEnvironmentWriteError, wantedCode: errs.CodeEnvironmentInUse},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var biz *apperr.BizError
			require.ErrorAs(t, tc.mapper(tc.err), &biz)
			require.Equal(t, tc.wantedCode, biz.Code)
		})
	}
	sentinel := errors.New("database unavailable")
	require.ErrorIs(t, mapProjectWriteError(sentinel), sentinel)
	require.ErrorIs(t, mapEnvironmentWriteError(sentinel), sentinel)
	otherConstraint := &pgconn.PgError{Code: "23505", ConstraintName: "other"}
	require.ErrorIs(t, mapEnvironmentWriteError(otherConstraint), otherConstraint)
}

func TestRepositoryNotFoundMappings(t *testing.T) {
	var biz *apperr.BizError
	require.ErrorAs(t, mapProjectReadError(gorm.ErrRecordNotFound), &biz)
	require.Equal(t, errs.CodeProjectNotFound, biz.Code)
	require.ErrorAs(t, mapEnvironmentReadError(gorm.ErrRecordNotFound), &biz)
	require.Equal(t, errs.CodeEnvironmentNotFound, biz.Code)
	sentinel := errors.New("database unavailable")
	require.ErrorIs(t, mapProjectReadError(sentinel), sentinel)
	require.ErrorIs(t, mapEnvironmentReadError(sentinel), sentinel)
}

func TestPaginationBounds(t *testing.T) {
	page, size, offset, err := pageValues(0, 0)
	require.NoError(t, err)
	require.Equal(t, []int{1, 20, 0}, []int{page, size, offset})
	page, size, offset, err = pageValues(2, 1000)
	require.NoError(t, err)
	require.Equal(t, []int{2, 100, 100}, []int{page, size, offset})
	_, _, _, err = pageValues(int(^uint(0)>>1), 2)
	require.Equal(t, apperr.CodeValidation, bizCode(t, err))
}
