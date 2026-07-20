//go:build dbtest

package httpcredential_test

import (
	"context"
	"github.com/stretchr/testify/require"
	"optimus-be/internal/infra/db"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/credentials/httpcredential"
	"path/filepath"
	"testing"
	"time"
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

func TestParentDeleteLockSerializesDatasourceInsert(t *testing.T) {
	g, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	defer td()
	r := httpcredential.NewRepo(g)
	credential := &models.HTTPCredential{Name: "locked", AuthType: "bearer", SecretCiphertext: []byte("x")}
	require.NoError(t, r.Create(t.Context(), credential))
	tx := g.Begin()
	require.NoError(t, tx.Error)
	_, err := r.GetForUpdate(t.Context(), tx, credential.ID)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
	defer cancel()
	id := credential.ID
	child := &models.ObservabilityDatasource{Name: "blocked", BaseURL: "https://prom.example", AuthType: "bearer", HTTPCredentialID: &id}
	err = g.WithContext(ctx).Create(child).Error
	require.Error(t, err)
	require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
	require.NoError(t, tx.Rollback().Error)
	var children int64
	require.NoError(t, g.Model(&models.ObservabilityDatasource{}).Where("http_credential_id=?", credential.ID).Count(&children).Error)
	require.Zero(t, children)
	_, err = r.Get(t.Context(), credential.ID)
	require.NoError(t, err)
}
