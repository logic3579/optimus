//go:build dbtest

package auth_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/auth"
)

func TestLogout_RevokesGivenToken(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	seedUserWithPassword(t, gdb, "alice", "s3cret")
	svc := mkSvcStd(t, gdb)
	pair := loginAlice(t, svc)

	require.NoError(t, svc.Logout(context.Background(), pair.RefreshToken))

	var row models.RefreshToken
	gdb.Where("token_hash = ?", auth.Sha256HexForTest(pair.RefreshToken)).First(&row)
	require.NotNil(t, row.RevokedAt)
}

func TestLogout_IsIdempotentForUnknownToken(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	svc := mkSvcStd(t, gdb)
	require.NoError(t, svc.Logout(context.Background(), "nonexistent"))
}
