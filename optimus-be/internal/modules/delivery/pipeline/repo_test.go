package pipeline

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/delivery/errs"
)

func TestPipelineRepositoryRejectsUnsupportedExecutor(t *testing.T) {
	err := validatePipelineStages([]models.DeliveryPipelineStage{{Executor: models.DeliveryExecutor("shell")}})
	require.Equal(t, errs.CodePipelineInvalid, businessCode(t, err))
}

func TestPipelineRepositoryMapsPublicationConflicts(t *testing.T) {
	for _, pgErr := range []*pgconn.PgError{
		{Code: "55P03"},
		{Code: "23505", ConstraintName: "delivery_pipelines_project_version_unique"},
		{Code: "23505", ConstraintName: "delivery_pipelines_current_project_unique"},
	} {
		require.Equal(t, errs.CodePipelineVersionConflict, businessCode(t, mapPipelineWriteError(pgErr)))
	}
	sentinel := errors.New("database unavailable")
	require.ErrorIs(t, mapPipelineWriteError(sentinel), sentinel)
	var business *apperr.BizError
	require.False(t, errors.As(mapPipelineWriteError(&pgconn.PgError{Code: "23505", ConstraintName: "other"}), &business))
}
