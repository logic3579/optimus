//go:build dbtest

package runs

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/models"
	"optimus-be/tests/dbtest"
)

func TestRepoInsertFinishAndPrune(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "..", "migrations"))
	t.Cleanup(teardown)
	key := dbtest.SeedCloudKey(t, gdb, "runs-key")
	account := dbtest.SeedCloudAccount(t, gdb, key.ID, "runs-account", "us-east-1")
	repo := NewRepo(gdb)

	id, err := repo.Insert(context.Background(), InsertRequest{
		CloudAccountID: account.ID,
		Region:         "us-east-1",
		ResourceType:   "instance",
		Trigger:        "test",
	})
	require.NoError(t, err)
	require.NotZero(t, id)

	require.NoError(t, repo.Finish(context.Background(), id, FinishRequest{
		Status:           "success",
		ItemsSeen:        2,
		ItemsSoftDeleted: 1,
	}))
	var row models.AssetsSyncRun
	require.NoError(t, gdb.First(&row, id).Error)
	require.NotNil(t, row.FinishedAt)
	require.Equal(t, "success", row.Status)
	require.EqualValues(t, 2, row.ItemsSeen)
	require.EqualValues(t, 1, row.ItemsSoftDeleted)
	require.Error(t, repo.Finish(context.Background(), id, FinishRequest{Status: "failed"}))
	require.Error(t, repo.Finish(context.Background(), id+999, FinishRequest{Status: "failed"}))

	require.NoError(t, gdb.Model(&row).Update("started_at", time.Now().AddDate(0, 0, -31)).Error)
	pruned, err := repo.Prune(context.Background(), 30)
	require.NoError(t, err)
	require.EqualValues(t, 1, pruned)
}
