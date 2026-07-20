//go:build dbtest

package httpcredential_test

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
	"optimus-be/internal/modules/credentials/httpcredential"
	"optimus-be/internal/modules/observability/datasource"
	dsinuse "optimus-be/internal/modules/observability/datasource/inuse"
)

func TestRepoFiltersAndSoftDelete(t *testing.T) {
	g, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	defer td()
	r := httpcredential.NewRepo(g)
	u := "u"
	require.NoError(t, r.Create(t.Context(), &models.HTTPCredential{Name: "one", AuthType: "basic", Username: &u, SecretCiphertext: []byte("x")}))
	require.NoError(t, r.Create(t.Context(), &models.HTTPCredential{Name: "two", AuthType: "bearer", SecretCiphertext: []byte("y")}))
	rows, total, err := r.List(t.Context(), httpcredential.ListQuery{AuthType: "basic"})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	require.NoError(t, r.Delete(t.Context(), rows[0].ID))
	_, err = r.Get(t.Context(), rows[0].ID)
	require.Error(t, err)
}

type blockingCredentialCounter struct {
	delegate *dsinuse.Counter
	entered  chan struct{}
	release  chan struct{}
	once     sync.Once
}

func (c *blockingCredentialCounter) CountByHTTPCredentialID(ctx context.Context, id uint64) (int64, error) {
	return c.delegate.CountByHTTPCredentialID(ctx, id)
}

func (c *blockingCredentialCounter) CountByHTTPCredentialIDTx(ctx context.Context, tx *gorm.DB, id uint64) (int64, error) {
	c.once.Do(func() { close(c.entered) })
	select {
	case <-c.release:
	case <-ctx.Done():
		return 0, ctx.Err()
	}
	return c.delegate.CountByHTTPCredentialIDTx(ctx, tx, id)
}

func seedRaceUser(t *testing.T, g *gorm.DB, name string) *models.User {
	t.Helper()
	u := &models.User{Username: name, Email: name + "@example.test", PasswordHash: "hash"}
	require.NoError(t, g.Create(u).Error)
	return u
}

func requireAuditCount(t *testing.T, g *gorm.DB, action string, targetID uint64, want int64) {
	t.Helper()
	var n int64
	require.NoError(t, g.Model(&models.AuditLog{}).Where("action = ? AND target_id = ?", action, strconv.FormatUint(targetID, 10)).Count(&n).Error)
	require.Equal(t, want, n)
}

func TestServiceDeleteWinsRaceAgainstDatasourceCreate(t *testing.T) {
	g, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	defer td()
	r := httpcredential.NewRepo(g)
	actor := seedRaceUser(t, g, "credential-delete-first")
	credential := &models.HTTPCredential{Name: "locked-delete-first", AuthType: "bearer", SecretCiphertext: []byte("x"), CreatedByUserID: &actor.ID}
	require.NoError(t, r.Create(t.Context(), credential))
	recorder := audit.NewRecorder(g)
	realCounter := dsinuse.New(g)
	gate := &blockingCredentialCounter{delegate: realCounter, entered: make(chan struct{}), release: make(chan struct{})}
	credentialService := httpcredential.NewService(r, nil, recorder)
	credentialService.SetInUseCounter(gate)
	dsRepo := datasource.NewRepo(g)
	datasourceService := datasource.NewService(dsRepo, dsRepo, dsRepo, realCounter, nil, nil, recorder)

	deleteResult := make(chan error, 1)
	go func() {
		deleteResult <- credentialService.Delete(t.Context(), actor.ID, "192.0.2.1", "race-test", credential.ID)
	}()
	select {
	case <-gate.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("delete did not reach the transactional usage count")
	}
	createResult := make(chan error, 1)
	go func() {
		_, err := datasourceService.Create(t.Context(), actor.ID, "192.0.2.2", "race-test", datasource.CreateRequest{Name: "blocked-delete-first", BaseURL: "https://prom.example", AuthType: "bearer", HTTPCredentialID: &credential.ID})
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
	require.NoError(t, g.Model(&models.ObservabilityDatasource{}).Where("http_credential_id=?", credential.ID).Count(&children).Error)
	require.Zero(t, children)
	_, err := r.Get(t.Context(), credential.ID)
	require.Error(t, err)
	var stored models.HTTPCredential
	require.NoError(t, g.Unscoped().First(&stored, credential.ID).Error)
	require.True(t, stored.DeletedAt.Valid)
	requireAuditCount(t, g, "credentials.http_credential.delete", credential.ID, 1)
}

func TestDatasourceCommitWinsRaceAgainstServiceDelete(t *testing.T) {
	g, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	defer td()
	r := httpcredential.NewRepo(g)
	actor := seedRaceUser(t, g, "credential-child-first")
	credential := &models.HTTPCredential{Name: "locked-child-first", AuthType: "bearer", SecretCiphertext: []byte("x"), CreatedByUserID: &actor.ID}
	require.NoError(t, r.Create(t.Context(), credential))
	recorder := audit.NewRecorder(g)
	service := httpcredential.NewService(r, nil, recorder)
	service.SetInUseCounter(dsinuse.New(g))

	tx := g.Begin()
	require.NoError(t, tx.Error)
	_, err := r.GetForUpdate(t.Context(), tx, credential.ID)
	require.NoError(t, err)
	child := &models.ObservabilityDatasource{Name: "committed-child-first", BaseURL: "https://prom.example", AuthType: "bearer", HTTPCredentialID: &credential.ID}
	require.NoError(t, tx.Create(child).Error)
	deleteResult := make(chan error, 1)
	go func() { deleteResult <- service.Delete(t.Context(), actor.ID, "192.0.2.3", "race-test", credential.ID) }()
	select {
	case err := <-deleteResult:
		t.Fatalf("delete did not block on the parent lock: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	require.NoError(t, tx.Commit().Error)
	err = <-deleteResult
	require.Error(t, err)
	require.False(t, errors.Is(err, context.DeadlineExceeded))
	_, err = r.Get(t.Context(), credential.ID)
	require.NoError(t, err)
	var children int64
	require.NoError(t, g.Model(&models.ObservabilityDatasource{}).Where("http_credential_id=?", credential.ID).Count(&children).Error)
	require.EqualValues(t, 1, children)
	requireAuditCount(t, g, "credentials.http_credential.delete", credential.ID, 0)
}
