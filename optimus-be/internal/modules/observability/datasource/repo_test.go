//go:build dbtest

package datasource

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"optimus-be/internal/infra/db"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/credentials"
	"optimus-be/internal/modules/credentials/httpcredential"
	"optimus-be/tests/dbtest"
)

type passthroughCipher struct{}

func (passthroughCipher) Seal(v []byte) ([]byte, error) { return append([]byte(nil), v...), nil }
func (passthroughCipher) Open(v []byte) ([]byte, error) { return append([]byte(nil), v...), nil }

type captureTester struct{ auth string }

func (t *captureTester) Test(_ context.Context, _ Detail, c *credentials.HTTPCredential) (map[string]string, error) {
	t.auth = c.AuthType
	return map[string]string{"version": "test"}, nil
}

func TestRepoCRUDListFiltersAndCounts(t *testing.T) {
	gdb, done := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	t.Cleanup(done)
	ctx := context.Background()
	r := NewRepo(gdb)
	user := dbtest.SeedUser(t, gdb, "ds-user")
	cred := dbtest.SeedHTTPCredential(t, gdb, user.ID, "prom-basic", "basic")
	cluster := dbtest.SeedCluster(t, gdb, "prod")
	one := &models.ObservabilityDatasource{Name: "prod-prom", BaseURL: "https://prom.example.com", AuthType: "basic", HTTPCredentialID: &cred.ID, ClusterID: &cluster.ID, CreatedByUserID: &user.ID}
	two := &models.ObservabilityDatasource{Name: "dev-prom", BaseURL: "https://dev.example.com", AuthType: "none"}
	require.NoError(t, r.Create(ctx, one))
	require.NoError(t, r.Create(ctx, two))
	items, n, err := r.List(ctx, ListQuery{Q: "prod", AuthType: "basic", ClusterID: &cluster.ID, Page: 1, PageSize: 1})
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
	require.Len(t, items, 1)
	require.Equal(t, "prom-basic", items[0].HTTPCredential.Name)
	require.False(t, items[0].HasCustomCA)
	c, err := r.CountByHTTPCredentialID(ctx, cred.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, c)
	c, err = r.CountByClusterID(ctx, cluster.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, c)
	require.NoError(t, r.SoftDelete(ctx, one.ID))
	c, err = r.CountByClusterID(ctx, cluster.ID)
	require.NoError(t, err)
	require.Zero(t, c)
}

func TestRepoMapsActiveNameConflict(t *testing.T) {
	gdb, done := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	t.Cleanup(done)
	r := NewRepo(gdb)
	ctx := context.Background()
	require.NoError(t, r.Create(ctx, &models.ObservabilityDatasource{Name: "same", BaseURL: "https://a.example", AuthType: "none"}))
	err := r.Create(ctx, &models.ObservabilityDatasource{Name: "same", BaseURL: "https://b.example", AuthType: "none"})
	code(t, err, 44002)
}

func TestConnectionConsumesBasicAndBearerWithActorAttribution(t *testing.T) {
	gdb, done := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	t.Cleanup(done)
	user := dbtest.SeedUser(t, gdb, "consume-actor")
	recorder := audit.NewRecorder(gdb)
	httpSvc := httpcredential.NewService(httpcredential.NewRepo(gdb), passthroughCipher{}, recorder)
	consumer := credentials.NewConsumer(nil, nil, nil, httpSvc)
	for _, authType := range []string{"basic", "bearer"} {
		t.Run(authType, func(t *testing.T) {
			var username *string
			if authType == "basic" {
				value := "metrics"
				username = &value
			}
			credential, err := httpSvc.Create(t.Context(), user.ID, "", "", httpcredential.CreateRequest{Name: "consume-" + authType, AuthType: authType, Username: username, Secret: "secret-" + authType})
			require.NoError(t, err)
			require.NoError(t, gdb.Where("action = ?", "credentials.consume.http_credential").Delete(&models.AuditLog{}).Error)
			r := &fakeRepo{row: &models.ObservabilityDatasource{ID: credential.ID + 100, Name: "prom", BaseURL: "https://example.com", AuthType: authType, HTTPCredentialID: &credential.ID}}
			tester := &captureTester{}
			svc := newServiceForTest(r, &fakeMeta{}, &fakeCluster{ok: true}, &fakePanels{}, consumer, tester, &fakeAudit{})
			result, err := svc.TestConnection(t.Context(), user.ID, "", "", r.row.ID)
			require.NoError(t, err)
			require.True(t, result.Reachable)
			require.Equal(t, authType, tester.auth)
			var entry models.AuditLog
			require.NoError(t, gdb.Where("action = ?", "credentials.consume.http_credential").Order("id DESC").First(&entry).Error)
			require.NotNil(t, entry.UserID)
			require.Equal(t, user.ID, *entry.UserID)
			var payload map[string]any
			require.NoError(t, json.Unmarshal(entry.Payload, &payload))
			require.Equal(t, "observability.datasource.test", payload["purpose"])
		})
	}
}

func TestReferenceLookupsLockParentsAgainstConcurrentSoftDelete(t *testing.T) {
	gdb, done := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	t.Cleanup(done)
	user := dbtest.SeedUser(t, gdb, "lock-user")
	credential := dbtest.SeedHTTPCredential(t, gdb, user.ID, "lock-credential", "bearer")
	cluster := dbtest.SeedCluster(t, gdb, "lock-cluster")
	r := NewRepo(gdb)
	tx := gdb.Begin()
	require.NoError(t, tx.Error)
	t.Cleanup(func() { tx.Rollback() })
	_, err := r.GetHTTPMetadataTx(t.Context(), tx, credential.ID)
	require.NoError(t, err)
	ok, err := r.ExistsTx(t.Context(), tx, cluster.ID)
	require.NoError(t, err)
	require.True(t, ok)
	for name, row := range map[string]any{"credential": credential, "cluster": cluster} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
			defer cancel()
			err := gdb.WithContext(ctx).Delete(row).Error
			require.Error(t, err)
			require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
			require.Contains(t, err.Error(), "context deadline exceeded")
		})
	}
}
