//go:build dbtest

package integration_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/models"
	assetsync "optimus-be/internal/modules/assets/sync"
	"optimus-be/internal/modules/assets/sync/runs"
	"optimus-be/internal/modules/credentials"
	"optimus-be/tests/dbtest"
)

type integrationFetcher struct {
	mu        sync.RWMutex
	instances []models.AWSInstance
	err       error
}

func (f *integrationFetcher) setInstances(items ...models.AWSInstance) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.instances = append([]models.AWSInstance(nil), items...)
	f.err = nil
}

func (f *integrationFetcher) setError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func (f *integrationFetcher) FetchInstances(context.Context, *assetsync.Clients) ([]models.AWSInstance, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return append([]models.AWSInstance(nil), f.instances...), f.err
}

func (*integrationFetcher) FetchVPCsAndSubnets(context.Context, *assetsync.Clients) ([]models.AWSVPC, []models.AWSSubnet, error) {
	return []models.AWSVPC{}, []models.AWSSubnet{}, nil
}

func (*integrationFetcher) FetchDatabases(context.Context, *assetsync.Clients) ([]models.AWSDatabase, error) {
	return []models.AWSDatabase{}, nil
}

type integrationClientFactory struct{}

func (integrationClientFactory) For(context.Context, *credentials.CloudKey, string, time.Duration) (*assetsync.Clients, error) {
	return &assetsync.Clients{}, nil
}

type integrationCloudKeyConsumer struct{}

func (integrationCloudKeyConsumer) GetCloudKey(context.Context, uint64, string) (*credentials.CloudKey, error) {
	return &credentials.CloudKey{Provider: "aws", AccessKeyID: "integration-access", SecretAccessKey: "integration-secret"}, nil
}

func newIntegrationEngine(gdb *gorm.DB, fetcher assetsync.Fetcher) *assetsync.Engine {
	return assetsync.NewEngine(gdb, runs.NewRepo(gdb), integrationClientFactory{}, fetcher, integrationCloudKeyConsumer{}, time.Second)
}

func TestAssetsSyncEngineIntegration(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	t.Cleanup(teardown)
	key := dbtest.SeedCloudKey(t, gdb, "assets-sync-key")
	account := dbtest.SeedCloudAccount(t, gdb, key.ID, "assets-sync-account", "us-east-1")
	fetcher := &integrationFetcher{}
	engine := newIntegrationEngine(gdb, fetcher)

	t.Run("successful authoritative sweeps persist and soft delete", func(t *testing.T) {
		fetcher.setInstances(
			models.AWSInstance{InstanceID: "i-x", Name: "keep"},
			models.AWSInstance{InstanceID: "i-y", Name: "remove"},
			models.AWSInstance{InstanceID: "i-z", Name: "remove"},
		)
		require.NoError(t, engine.RunAccount(context.Background(), account.ID, "cron", nil))

		var foundRuns []models.AssetsSyncRun
		require.NoError(t, gdb.Where("cloud_account_id = ?", account.ID).Order("id").Find(&foundRuns).Error)
		require.Len(t, foundRuns, 3)
		for _, run := range foundRuns {
			require.Equal(t, "success", run.Status)
		}
		require.Equal(t, []string{"instance", "network", "database"}, []string{
			foundRuns[0].ResourceType, foundRuns[1].ResourceType, foundRuns[2].ResourceType,
		})
		require.EqualValues(t, 3, foundRuns[0].ItemsSeen)
		var alive int64
		require.NoError(t, gdb.Model(&models.AWSInstance{}).Where("cloud_account_id = ?", account.ID).Count(&alive).Error)
		require.EqualValues(t, 3, alive)

		fetcher.setInstances(models.AWSInstance{InstanceID: "i-x", Name: "still-present"})
		require.NoError(t, engine.RunAccount(context.Background(), account.ID, "cron", nil))
		require.NoError(t, gdb.Model(&models.AWSInstance{}).Where("cloud_account_id = ?", account.ID).Count(&alive).Error)
		require.EqualValues(t, 1, alive)
		var live models.AWSInstance
		require.NoError(t, gdb.Where("cloud_account_id = ?", account.ID).First(&live).Error)
		require.Equal(t, "i-x", live.InstanceID)
		var deleted int64
		require.NoError(t, gdb.Unscoped().Model(&models.AWSInstance{}).
			Where("cloud_account_id = ? AND deleted_at IS NOT NULL", account.ID).Count(&deleted).Error)
		require.EqualValues(t, 2, deleted)
	})

	t.Run("failed fetch records failure without deleting existing rows", func(t *testing.T) {
		existing := models.AWSInstance{
			CloudAccountID: account.ID, Region: "us-east-1", InstanceID: "i-existing", LastSeenAt: time.Now(),
		}
		require.NoError(t, gdb.Create(&existing).Error)
		fetcher.setError(errors.New("simulated upstream failure"))
		require.NoError(t, engine.RunAccount(context.Background(), account.ID, "cron", nil))

		var latest models.AssetsSyncRun
		require.NoError(t, gdb.Where("cloud_account_id = ? AND resource_type = ?", account.ID, "instance").Order("id DESC").First(&latest).Error)
		require.Equal(t, "failed", latest.Status)
		var alive int64
		require.NoError(t, gdb.Model(&models.AWSInstance{}).Where("cloud_account_id = ? AND instance_id = ?", account.ID, "i-existing").Count(&alive).Error)
		require.EqualValues(t, 1, alive)
	})

	t.Run("prune zero is no-op and positive retention deletes old runs", func(t *testing.T) {
		old := models.AssetsSyncRun{
			CloudAccountID: account.ID, Region: "us-east-1", ResourceType: "instance",
			StartedAt: time.Now().Add(-48 * time.Hour), Status: "success", Trigger: "cron",
		}
		require.NoError(t, gdb.Create(&old).Error)
		count, err := engine.PruneSyncRuns(context.Background(), 0)
		require.NoError(t, err)
		require.Zero(t, count)
		require.NoError(t, gdb.First(&models.AssetsSyncRun{}, old.ID).Error)

		count, err = engine.PruneSyncRuns(context.Background(), 1)
		require.NoError(t, err)
		require.EqualValues(t, 1, count)
		require.ErrorIs(t, gdb.First(&models.AssetsSyncRun{}, old.ID).Error, gorm.ErrRecordNotFound)
	})
}

func TestAssetsManualSyncHTTPQueuesAndEventuallyRuns(t *testing.T) {
	fetcher := &integrationFetcher{}
	fetcher.setInstances(models.AWSInstance{InstanceID: "i-manual"})
	r, gdb := setupServerWithAssetsWiring(t, &assetsTestWiring{
		Factory: integrationClientFactory{}, Fetcher: fetcher, Consumer: integrationCloudKeyConsumer{},
	})
	token := login(t, r, "admin", "S3cret-Pass!")
	key := dbtest.SeedCloudKey(t, gdb, "assets-manual-key")
	account := dbtest.SeedCloudAccount(t, gdb, key.ID, "assets-manual-account", "us-east-1")

	rec := assetsRequest(t, r, token, http.MethodPost, fmt.Sprintf("/api/v1/assets/cloud-accounts/%d/sync", account.ID), nil)
	require.Equalf(t, http.StatusOK, rec.Code, "manual sync failed: %s", rec.Body.String())
	require.Equal(t, true, bodyMap(t, rec)["data"].(map[string]any)["queued"])

	require.Eventually(t, func() bool {
		var count int64
		err := gdb.Model(&models.AssetsSyncRun{}).
			Where("cloud_account_id = ? AND trigger = ? AND status = ? AND finished_at IS NOT NULL", account.ID, "manual", "success").
			Count(&count).Error
		return err == nil && count == 3
	}, 2*time.Second, 20*time.Millisecond)
	var auditCount int64
	require.NoError(t, gdb.Model(&models.AuditLog{}).
		Where("action = ? AND target_id = ?", "assets.cloud_account.sync_trigger", fmt.Sprint(account.ID)).
		Count(&auditCount).Error)
	require.EqualValues(t, 1, auditCount)
}
