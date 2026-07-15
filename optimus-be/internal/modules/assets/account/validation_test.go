package account

import (
	"errors"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/modules/assets/errs"
)

func TestValidateRegionsRejectsDuplicates(t *testing.T) {
	err := validateRegions([]string{"us-east-1", "us-east-1"})
	var bizErr *apperr.BizError
	require.True(t, errors.As(err, &bizErr))
	require.Equal(t, errs.CodeAssetsRegionInvalid, bizErr.Code)
}

func TestNormalizeName(t *testing.T) {
	name, err := normalizeName("  production  ")
	require.NoError(t, err)
	require.Equal(t, "production", name)
	_, err = normalizeName("   ")
	var bizErr *apperr.BizError
	require.True(t, errors.As(err, &bizErr))
	require.Equal(t, apperr.CodeValidation, bizErr.Code)
}

func TestPaginationOffsetRejectsOverflow(t *testing.T) {
	_, _, _, err := paginationOffset(math.MaxInt, 200)
	var bizErr *apperr.BizError
	require.True(t, errors.As(err, &bizErr))
	require.Equal(t, apperr.CodeValidation, bizErr.Code)
	page, size, offset, err := paginationOffset(0, 0)
	require.NoError(t, err)
	require.Equal(t, 1, page)
	require.Equal(t, 20, size)
	require.Zero(t, offset)
}
