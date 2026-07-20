//go:build dbtest

package httpcredential_test

import (
	"github.com/stretchr/testify/require"
	"optimus-be/internal/infra/db"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/credentials/httpcredential"
	"path/filepath"
	"testing"
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
