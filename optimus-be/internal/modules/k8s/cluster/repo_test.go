//go:build dbtest

package cluster_test

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/k8s/cluster"
	"optimus-be/internal/modules/observability/datasource"
	dsinuse "optimus-be/internal/modules/observability/datasource/inuse"
)

func newRepo(t *testing.T) (*cluster.Repo, func()) {
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	return cluster.NewRepo(gdb), td
}

type blockingClusterCounter struct {
	delegate *dsinuse.Counter
	entered  chan struct{}
	release  chan struct{}
	once     sync.Once
}

func (c *blockingClusterCounter) CountByClusterID(ctx context.Context, id uint64) (int64, error) {
	return c.delegate.CountByClusterID(ctx, id)
}

func (c *blockingClusterCounter) CountByClusterIDTx(ctx context.Context, tx *gorm.DB, id uint64) (int64, error) {
	c.once.Do(func() { close(c.entered) })
	select {
	case <-c.release:
	case <-ctx.Done():
		return 0, ctx.Err()
	}
	return c.delegate.CountByClusterIDTx(ctx, tx, id)
}

func seedClusterRaceUser(t *testing.T, g *gorm.DB, name string) *models.User {
	t.Helper()
	u := &models.User{Username: name, Email: name + "@example.test", PasswordHash: "hash"}
	require.NoError(t, g.Create(u).Error)
	return u
}

func requireClusterAuditCount(t *testing.T, g *gorm.DB, action string, targetID uint64, want int64) {
	t.Helper()
	var n int64
	require.NoError(t, g.Model(&models.AuditLog{}).Where("action = ? AND target_id = ?", action, strconv.FormatUint(targetID, 10)).Count(&n).Error)
	require.Equal(t, want, n)
}

func TestServiceDeleteWinsRaceAgainstDatasourceCreate(t *testing.T) {
	repo, td := newRepo(t)
	defer td()
	kcID := setupKubeconfigRow(t, repo)
	actor := seedClusterRaceUser(t, repo.DB(), "cluster-delete-first")
	parent := &models.Cluster{Name: "locked-delete-first", KubeconfigID: kcID, Context: "ctx", Tags: []string{}}
	require.NoError(t, repo.Create(t.Context(), parent))
	recorder := audit.NewRecorder(repo.DB())
	realCounter := dsinuse.New(repo.DB())
	gate := &blockingClusterCounter{delegate: realCounter, entered: make(chan struct{}), release: make(chan struct{})}
	clusterService := cluster.NewService(repo, nil, nil, recorder)
	clusterService.SetObservabilityCounter(gate)
	dsRepo := datasource.NewRepo(repo.DB())
	datasourceService := datasource.NewService(dsRepo, dsRepo, dsRepo, realCounter, nil, nil, recorder)

	deleteResult := make(chan error, 1)
	go func() {
		deleteResult <- clusterService.Delete(t.Context(), actor.ID, "192.0.2.4", "race-test", parent.ID)
	}()
	select {
	case <-gate.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("delete did not reach the transactional usage count")
	}
	createResult := make(chan error, 1)
	go func() {
		_, err := datasourceService.Create(t.Context(), actor.ID, "192.0.2.5", "race-test", datasource.CreateRequest{Name: "blocked-cluster-delete-first", BaseURL: "https://prom.example", AuthType: "none", ClusterID: &parent.ID})
		createResult <- err
	}()
	select {
	case err := <-createResult:
		t.Fatalf("datasource create did not block on the parent lock: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	close(gate.release)
	require.NoError(t, <-deleteResult)
	require.Error(t, <-createResult)

	var children int64
	require.NoError(t, repo.DB().Model(&models.ObservabilityDatasource{}).Where("cluster_id=?", parent.ID).Count(&children).Error)
	require.Zero(t, children)
	_, err := repo.Get(t.Context(), parent.ID)
	require.Error(t, err)
	var stored models.Cluster
	require.NoError(t, repo.DB().Unscoped().First(&stored, parent.ID).Error)
	require.True(t, stored.DeletedAt.Valid)
	requireClusterAuditCount(t, repo.DB(), "k8s.cluster.delete", parent.ID, 1)
}

func TestDatasourceCommitWinsRaceAgainstServiceDelete(t *testing.T) {
	repo, td := newRepo(t)
	defer td()
	kcID := setupKubeconfigRow(t, repo)
	actor := seedClusterRaceUser(t, repo.DB(), "cluster-child-first")
	parent := &models.Cluster{Name: "locked-child-first", KubeconfigID: kcID, Context: "ctx", Tags: []string{}}
	require.NoError(t, repo.Create(t.Context(), parent))
	recorder := audit.NewRecorder(repo.DB())
	service := cluster.NewService(repo, nil, nil, recorder)
	service.SetObservabilityCounter(dsinuse.New(repo.DB()))

	tx := repo.DB().Begin()
	require.NoError(t, tx.Error)
	_, err := repo.GetForUpdate(t.Context(), tx, parent.ID)
	require.NoError(t, err)
	child := &models.ObservabilityDatasource{Name: "committed-cluster-child-first", BaseURL: "https://prom.example", AuthType: "none", ClusterID: &parent.ID}
	require.NoError(t, tx.Create(child).Error)
	deleteResult := make(chan error, 1)
	go func() { deleteResult <- service.Delete(t.Context(), actor.ID, "192.0.2.6", "race-test", parent.ID) }()
	select {
	case err := <-deleteResult:
		t.Fatalf("delete did not block on the parent lock: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	require.NoError(t, tx.Commit().Error)
	err = <-deleteResult
	require.Error(t, err)
	require.False(t, errors.Is(err, context.DeadlineExceeded))
	_, err = repo.Get(t.Context(), parent.ID)
	require.NoError(t, err)
	var children int64
	require.NoError(t, repo.DB().Model(&models.ObservabilityDatasource{}).Where("cluster_id=?", parent.ID).Count(&children).Error)
	require.EqualValues(t, 1, children)
	requireClusterAuditCount(t, repo.DB(), "k8s.cluster.delete", parent.ID, 0)
}

func setupKubeconfigRow(t *testing.T, repo *cluster.Repo) uint64 {
	t.Helper()
	kc := &models.CredentialKubeconfig{
		Name:          "kc-" + t.Name(),
		KubeconfigEnc: []byte{0x01, 0x02, 0x03},
	}
	require.NoError(t, repo.DB().Create(kc).Error)
	return kc.ID
}

func TestRepo_CreateAndGet(t *testing.T) {
	repo, td := newRepo(t)
	defer td()
	kcID := setupKubeconfigRow(t, repo)

	m := &models.Cluster{
		Name:         "prod",
		KubeconfigID: kcID,
		Context:      "prod-ctx",
		Tags:         []string{"prod", "us-east-1"},
	}
	require.NoError(t, repo.Create(context.Background(), m))
	require.NotZero(t, m.ID)

	got, err := repo.Get(context.Background(), m.ID)
	require.NoError(t, err)
	require.Equal(t, "prod", got.Name)
	require.Equal(t, []string{"prod", "us-east-1"}, []string(got.Tags))
	require.NotNil(t, got.Kubeconfig)
	require.Equal(t, kcID, got.Kubeconfig.ID)
}

func TestRepo_NameUniquePartial(t *testing.T) {
	repo, td := newRepo(t)
	defer td()
	kcID := setupKubeconfigRow(t, repo)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, &models.Cluster{
		Name: "n", KubeconfigID: kcID, Context: "c1",
	}))
	err := repo.Create(ctx, &models.Cluster{
		Name: "n", KubeconfigID: kcID, Context: "c2",
	})
	require.Error(t, err) // partial unique on name violates

	// soft-delete and re-create with same name should succeed
	m, err := repo.FindByName(ctx, "n")
	require.NoError(t, err)
	require.NoError(t, repo.Delete(ctx, m.ID))
	require.NoError(t, repo.Create(ctx, &models.Cluster{
		Name: "n", KubeconfigID: kcID, Context: "c3",
	}))
}

func TestRepo_KubeconfigContextUnique(t *testing.T) {
	repo, td := newRepo(t)
	defer td()
	kcID := setupKubeconfigRow(t, repo)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, &models.Cluster{
		Name: "a", KubeconfigID: kcID, Context: "shared",
	}))
	err := repo.Create(ctx, &models.Cluster{
		Name: "b", KubeconfigID: kcID, Context: "shared",
	})
	require.Error(t, err) // (kubeconfig_id, context) partial unique
}

func TestRepo_FK_Restrict(t *testing.T) {
	repo, td := newRepo(t)
	defer td()
	kcID := setupKubeconfigRow(t, repo)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, &models.Cluster{
		Name: "fk", KubeconfigID: kcID, Context: "c",
	}))
	err := repo.DB().Delete(&models.CredentialKubeconfig{}, kcID).Error
	require.Error(t, err) // ON DELETE RESTRICT
}

func TestRepo_TagFilter(t *testing.T) {
	repo, td := newRepo(t)
	defer td()
	kcID := setupKubeconfigRow(t, repo)
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, &models.Cluster{
		Name: "p", KubeconfigID: kcID, Context: "p", Tags: []string{"prod"},
	}))
	require.NoError(t, repo.Create(ctx, &models.Cluster{
		Name: "s", KubeconfigID: kcID, Context: "s", Tags: []string{"staging"},
	}))
	rows, _, err := repo.List(ctx, cluster.ListQuery{Tag: "prod"})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "p", rows[0].Name)
}

func TestRepo_CountByKubeconfigID(t *testing.T) {
	repo, td := newRepo(t)
	defer td()
	kcID := setupKubeconfigRow(t, repo)
	ctx := context.Background()
	n, err := repo.CountByKubeconfigID(ctx, kcID)
	require.NoError(t, err)
	require.EqualValues(t, 0, n)
	require.NoError(t, repo.Create(ctx, &models.Cluster{
		Name: "x", KubeconfigID: kcID, Context: "c",
	}))
	n, err = repo.CountByKubeconfigID(ctx, kcID)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
}
