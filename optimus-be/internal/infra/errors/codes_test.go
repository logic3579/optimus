package errors_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	apperr "optimus-be/internal/infra/errors"
)

func TestP2_NewCodesAreDistinct(t *testing.T) {
	codes := map[apperr.Code]string{
		apperr.CodeClusterUnreachable:    "CodeClusterUnreachable",
		apperr.CodeAPIServerForbidden:    "CodeAPIServerForbidden",
		apperr.CodeAPIServerUnauthorized: "CodeAPIServerUnauthorized",
		apperr.CodeAPIServerOther:        "CodeAPIServerOther",
		apperr.CodeLogUnavailable:        "CodeLogUnavailable",
	}
	require.Equal(t, 5, len(codes), "duplicate numeric codes")
	for c, name := range codes {
		require.NotZero(t, int(c), "%s must be nonzero", name)
	}
	require.Equal(t, apperr.Code(41101), apperr.CodeClusterUnreachable)
	require.Equal(t, apperr.Code(41103), apperr.CodeAPIServerForbidden)
	require.Equal(t, apperr.Code(41104), apperr.CodeAPIServerUnauthorized)
	require.Equal(t, apperr.Code(41105), apperr.CodeAPIServerOther)
	require.Equal(t, apperr.Code(41202), apperr.CodeLogUnavailable)
}

func TestAppsCodes_DistinctAndNonZero(t *testing.T) {
	codes := []apperr.Code{
		apperr.CodeAppsApplicationInUse, apperr.CodeAppsChartRepoInUse,
		apperr.CodeAppsReleaseNameDuplicate, apperr.CodeAppsApplicationOnDeletedCluster,
		apperr.CodeAppsRepoUnreachable, apperr.CodeAppsRepoUnauthorized,
		apperr.CodeAppsRepoChartNotFound, apperr.CodeAppsRepoInvalidIndex,
		apperr.CodeAppsRepoOCIError, apperr.CodeAppsRepoOther,
		apperr.CodeAppsReleaseAlreadyExists, apperr.CodeAppsReleaseNotFound,
		apperr.CodeAppsReleaseHistoryTooShort, apperr.CodeAppsReleaseStillPresent,
		apperr.CodeAppsReleaseInvalidValues, apperr.CodeAppsReleaseOther,
	}
	seen := map[apperr.Code]bool{}
	for _, c := range codes {
		if c == 0 {
			t.Errorf("zero-valued code in apps block")
		}
		if seen[c] {
			t.Errorf("duplicate code %d in apps block", c)
		}
		seen[c] = true
	}
}
