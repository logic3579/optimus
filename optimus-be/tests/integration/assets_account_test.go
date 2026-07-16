//go:build dbtest

package integration_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	asseterrs "optimus-be/internal/modules/assets/errs"
	"optimus-be/tests/dbtest"
)

func assetsRequest(t *testing.T, r http.Handler, token, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	var req *http.Request
	if body == nil {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, mustJSONBody(t, body))
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(rec, req)
	return rec
}

func createCloudAccountHTTP(t *testing.T, r http.Handler, token string, keyID uint64, name string, regions ...string) (uint64, *httptest.ResponseRecorder) {
	t.Helper()
	rec := assetsRequest(t, r, token, http.MethodPost, "/api/v1/assets/cloud-accounts", map[string]any{
		"name": name, "provider": "aws", "cloudkey_id": keyID,
		"enabled_regions": regions, "description": "integration account",
	})
	require.Equalf(t, http.StatusOK, rec.Code, "create account failed: %s", rec.Body.String())
	data := bodyMap(t, rec)["data"].(map[string]any)
	return uint64(data["id"].(float64)), rec
}

func TestAssetsCloudAccountHTTPIntegration(t *testing.T) {
	r, gdb := setupServer(t)
	token := login(t, r, "admin", "S3cret-Pass!")
	key := dbtest.SeedCloudKey(t, gdb, "assets-http-key")

	t.Run("create persists detail and rejects invalid inputs", func(t *testing.T) {
		id, rec := createCloudAccountHTTP(t, r, token, key.ID, "primary-assets", "us-east-1", "eu-west-1")
		data := bodyMap(t, rec)["data"].(map[string]any)
		require.ElementsMatch(t, []any{"us-east-1", "eu-west-1"}, data["enabled_regions"].([]any))

		var row models.CloudAccount
		require.NoError(t, gdb.First(&row, id).Error)
		require.Equal(t, "primary-assets", row.Name)
		require.Equal(t, models.StringArray{"us-east-1", "eu-west-1"}, row.EnabledRegions)

		duplicate := assetsRequest(t, r, token, http.MethodPost, "/api/v1/assets/cloud-accounts", map[string]any{
			"name": "primary-assets", "provider": "aws", "cloudkey_id": key.ID, "enabled_regions": []string{"us-east-1"},
		})
		require.Equal(t, http.StatusUnprocessableEntity, duplicate.Code)
		require.EqualValues(t, asseterrs.CodeAssetsCloudAccountNameConflict, bodyMap(t, duplicate)["code"])

		unsupported := assetsRequest(t, r, token, http.MethodPost, "/api/v1/assets/cloud-accounts", map[string]any{
			"name": "gcp-account", "provider": "gcp", "cloudkey_id": key.ID, "enabled_regions": []string{"us-east-1"},
		})
		require.Equal(t, http.StatusUnprocessableEntity, unsupported.Code)
		require.EqualValues(t, asseterrs.CodeAssetsProviderUnsupported, bodyMap(t, unsupported)["code"])

		invalidRegion := assetsRequest(t, r, token, http.MethodPost, "/api/v1/assets/cloud-accounts", map[string]any{
			"name": "bad-region", "provider": "aws", "cloudkey_id": key.ID, "enabled_regions": []string{"not-a-region"},
		})
		require.Equal(t, http.StatusUnprocessableEntity, invalidRegion.Code)
		require.EqualValues(t, asseterrs.CodeAssetsRegionInvalid, bodyMap(t, invalidRegion)["code"])
	})

	t.Run("region shrink soft deletes resources only in removed regions", func(t *testing.T) {
		id, _ := createCloudAccountHTTP(t, r, token, key.ID, "region-shrink", "us-east-1", "eu-west-1")
		dbtest.SeedAWSInstance(t, gdb, id, "us-east-1", "i-east")
		dbtest.SeedAWSInstance(t, gdb, id, "eu-west-1", "i-west")

		rec := assetsRequest(t, r, token, http.MethodPut, fmt.Sprintf("/api/v1/assets/cloud-accounts/%d", id), map[string]any{
			"enabled_regions": []string{"us-east-1"},
		})
		require.Equalf(t, http.StatusOK, rec.Code, "region update failed: %s", rec.Body.String())

		var east, west models.AWSInstance
		require.NoError(t, gdb.Unscoped().Where("instance_id = ?", "i-east").First(&east).Error)
		require.NoError(t, gdb.Unscoped().Where("instance_id = ?", "i-west").First(&west).Error)
		require.False(t, east.DeletedAt.Valid)
		require.True(t, west.DeletedAt.Valid)
	})

	t.Run("delete cascades every resource type", func(t *testing.T) {
		id, _ := createCloudAccountHTTP(t, r, token, key.ID, "cascade-delete", "us-east-1")
		now := time.Now()
		require.NoError(t, gdb.Create(&models.AWSInstance{CloudAccountID: id, Region: "us-east-1", InstanceID: "i-cascade", LastSeenAt: now}).Error)
		require.NoError(t, gdb.Create(&models.AWSVPC{CloudAccountID: id, Region: "us-east-1", VPCID: "vpc-cascade", LastSeenAt: now}).Error)
		require.NoError(t, gdb.Create(&models.AWSSubnet{CloudAccountID: id, Region: "us-east-1", SubnetID: "subnet-cascade", VPCID: "vpc-cascade", LastSeenAt: now}).Error)
		require.NoError(t, gdb.Create(&models.AWSDatabase{CloudAccountID: id, Region: "us-east-1", DBInstanceID: "db-cascade", LastSeenAt: now}).Error)

		rec := assetsRequest(t, r, token, http.MethodDelete, fmt.Sprintf("/api/v1/assets/cloud-accounts/%d", id), nil)
		require.Equalf(t, http.StatusOK, rec.Code, "delete failed: %s", rec.Body.String())
		require.EqualValues(t, 4, bodyMap(t, rec)["data"].(map[string]any)["cascaded_resources_count"])

		var accountCount int64
		require.NoError(t, gdb.Unscoped().Model(&models.CloudAccount{}).Where("id = ? AND deleted_at IS NOT NULL", id).Count(&accountCount).Error)
		require.EqualValues(t, 1, accountCount)
		for _, model := range []any{&models.AWSInstance{}, &models.AWSVPC{}, &models.AWSSubnet{}, &models.AWSDatabase{}} {
			var count int64
			require.NoError(t, gdb.Unscoped().Model(model).Where("cloud_account_id = ? AND deleted_at IS NOT NULL", id).Count(&count).Error)
			require.EqualValues(t, 1, count, "%T was not soft-deleted", model)
		}
	})

	t.Run("cloud key delete is refused while referenced", func(t *testing.T) {
		blockedKey := dbtest.SeedCloudKey(t, gdb, "blocked-assets-key")
		_, _ = createCloudAccountHTTP(t, r, token, blockedKey.ID, "key-reference", "us-east-1")
		rec := assetsRequest(t, r, token, http.MethodDelete, fmt.Sprintf("/api/v1/credentials/cloud-keys/%d", blockedKey.ID), nil)
		require.Equal(t, http.StatusConflict, rec.Code)
		require.EqualValues(t, apperr.CodeAssetsCloudAccountInUse, bodyMap(t, rec)["code"])
		var row models.CredentialCloudKey
		require.NoError(t, gdb.First(&row, blockedKey.ID).Error)
	})
}
