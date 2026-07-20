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

func TestAssets43xxxCodesDistinct(t *testing.T) {
	codes := []struct {
		name     string
		code     apperr.Code
		expected apperr.Code
	}{
		{"CodeAssetsCloudAccountInUse", apperr.CodeAssetsCloudAccountInUse, 43001},
		{"CodeAssetsCloudAccountNotFound", apperr.CodeAssetsCloudAccountNotFound, 43002},
		{"CodeAssetsCloudAccountNameConflict", apperr.CodeAssetsCloudAccountNameConflict, 43003},
		{"CodeAssetsRegionInvalid", apperr.CodeAssetsRegionInvalid, 43004},
		{"CodeAssetsProviderUnsupported", apperr.CodeAssetsProviderUnsupported, 43005},
		{"CodeAssetsCloudAccountDisabled", apperr.CodeAssetsCloudAccountDisabled, 43006},
		{"CodeAssetsCloudKeyNotFound", apperr.CodeAssetsCloudKeyNotFound, 43008},
		{"CodeAssetsVPCNotFound", apperr.CodeAssetsVPCNotFound, 43009},
		{"CodeAssetsSyncBusy", apperr.CodeAssetsSyncBusy, 43101},
		{"CodeAssetsAWSUnauthorized", apperr.CodeAssetsAWSUnauthorized, 43102},
		{"CodeAssetsAWSForbidden", apperr.CodeAssetsAWSForbidden, 43103},
		{"CodeAssetsAWSUnreachable", apperr.CodeAssetsAWSUnreachable, 43104},
		{"CodeAssetsAWSThrottled", apperr.CodeAssetsAWSThrottled, 43105},
		{"CodeAssetsAWSOther", apperr.CodeAssetsAWSOther, 43106},
		{"CodeAssetsAWSConfig", apperr.CodeAssetsAWSConfig, 43107},
	}
	seen := map[apperr.Code]bool{}
	for _, tc := range codes {
		if tc.code != tc.expected {
			t.Errorf("%s = %d, want %d", tc.name, tc.code, tc.expected)
		}
		if tc.code == 0 {
			t.Errorf("%s is zero", tc.name)
		}
		if tc.code < 43000 || tc.code > 43999 {
			t.Errorf("%s code %d outside 43xxx segment", tc.name, tc.code)
		}
		if seen[tc.code] {
			t.Errorf("%s code %d duplicated", tc.name, tc.code)
		}
		seen[tc.code] = true
	}
	if len(codes) != 15 {
		t.Errorf("expected 15 P4 codes, got %d", len(codes))
	}
}
