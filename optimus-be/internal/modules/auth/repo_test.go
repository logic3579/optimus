//go:build dbtest

package auth_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/auth"
)

func seedUser(t *testing.T, gdb *gorm.DB, username string) uint64 {
	t.Helper()
	u := &models.User{Username: username, Email: username + "@x.io", PasswordHash: "x", Status: "enabled"}
	require.NoError(t, gdb.Create(u).Error)
	return u.ID
}

func TestRepo_CreateAndFindByHash(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	uid := seedUser(t, gdb, "alice")

	repo := auth.NewRepo(gdb)
	exp := time.Now().Add(7 * 24 * time.Hour)
	rt, err := repo.CreateRefreshToken(context.Background(), uid, "hashhash", exp, "ua", "1.1.1.1")
	require.NoError(t, err)
	require.NotZero(t, rt.ID)

	got, err := repo.FindRefreshTokenByHash(context.Background(), "hashhash")
	require.NoError(t, err)
	require.Equal(t, rt.ID, got.ID)
	require.Equal(t, uid, got.UserID)
}

func TestRepo_FindMissingReturnsError(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	repo := auth.NewRepo(gdb)
	_, err := repo.FindRefreshTokenByHash(context.Background(), "nope")
	require.Error(t, err)
}

func TestRepo_RevokeMarksRevokedAt(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	uid := seedUser(t, gdb, "alice")
	repo := auth.NewRepo(gdb)
	rt, _ := repo.CreateRefreshToken(context.Background(), uid, "h1", time.Now().Add(time.Hour), "", "")

	require.NoError(t, repo.RevokeRefreshToken(context.Background(), rt.ID))

	var got models.RefreshToken
	gdb.First(&got, rt.ID)
	require.NotNil(t, got.RevokedAt)
}

func TestRepo_RevokeAllForUserMarksAll(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	uid := seedUser(t, gdb, "alice")
	other := seedUser(t, gdb, "bob")
	repo := auth.NewRepo(gdb)
	_, _ = repo.CreateRefreshToken(context.Background(), uid, "h1", time.Now().Add(time.Hour), "", "")
	_, _ = repo.CreateRefreshToken(context.Background(), uid, "h2", time.Now().Add(time.Hour), "", "")
	_, _ = repo.CreateRefreshToken(context.Background(), other, "h3", time.Now().Add(time.Hour), "", "")

	require.NoError(t, repo.RevokeAllRefreshTokensForUser(context.Background(), uid))

	var revoked int64
	gdb.Model(&models.RefreshToken{}).Where("user_id = ? AND revoked_at IS NOT NULL", uid).Count(&revoked)
	require.Equal(t, int64(2), revoked)

	var otherActive int64
	gdb.Model(&models.RefreshToken{}).Where("user_id = ? AND revoked_at IS NULL", other).Count(&otherActive)
	require.Equal(t, int64(1), otherActive)
}
