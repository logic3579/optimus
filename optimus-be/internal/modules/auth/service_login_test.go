//go:build dbtest

package auth_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"optimus-be/internal/infra/crypto"
	"optimus-be/internal/infra/db"
	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/ratelimit"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/auth"
)

func mkSvc(t *testing.T, gdb *gorm.DB) *auth.Service {
	t.Helper()
	return auth.NewService(
		auth.NewRepo(gdb),
		crypto.NewJWTSigner("test_secret_must_be_at_least_32_bytes_!!"),
		ratelimit.NewLoginLimiter(5, time.Minute, time.Minute),
		auth.ServiceOptions{AccessTTL: 5 * time.Minute, RefreshTTL: 7 * 24 * time.Hour, BcryptCost: 4},
	)
}

func seedUserWithPassword(t *testing.T, gdb *gorm.DB, username, password string) uint64 {
	t.Helper()
	h, err := crypto.HashPassword(password, 4)
	require.NoError(t, err)
	u := &models.User{Username: username, Email: username + "@x.io", PasswordHash: h, Status: "enabled"}
	require.NoError(t, gdb.Create(u).Error)
	return u.ID
}

func TestLogin_SuccessReturnsTokens(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	uid := seedUserWithPassword(t, gdb, "alice", "s3cret")

	svc := mkSvc(t, gdb)
	pair, err := svc.Login(context.Background(), auth.LoginRequest{Username: "alice", Password: "s3cret"}, "1.1.1.1", "ua")
	require.NoError(t, err)
	require.NotEmpty(t, pair.AccessToken)
	require.NotEmpty(t, pair.RefreshToken)
	require.WithinDuration(t, time.Now().Add(5*time.Minute), pair.ExpiresAt, 5*time.Second)

	var u models.User
	gdb.First(&u, uid)
	require.NotNil(t, u.LastLoginAt)
}

func TestLogin_WrongPasswordReturnsInvalidCredentials(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	seedUserWithPassword(t, gdb, "alice", "s3cret")

	svc := mkSvc(t, gdb)
	_, err := svc.Login(context.Background(), auth.LoginRequest{Username: "alice", Password: "WRONG"}, "1.1.1.1", "ua")
	require.Error(t, err)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeInvalidCredentials, be.Code)
}

func TestLogin_UnknownUserReturnsInvalidCredentials(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	svc := mkSvc(t, gdb)
	_, err := svc.Login(context.Background(), auth.LoginRequest{Username: "nobody", Password: "x"}, "1.1.1.1", "ua")
	require.Error(t, err)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeInvalidCredentials, be.Code)
}

func TestLogin_RateLimitsExcessAttempts(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	seedUserWithPassword(t, gdb, "alice", "s3cret")

	svc := auth.NewService(
		auth.NewRepo(gdb),
		crypto.NewJWTSigner("test_secret_must_be_at_least_32_bytes_!!"),
		ratelimit.NewLoginLimiter(2, time.Minute, time.Minute),
		auth.ServiceOptions{AccessTTL: 5 * time.Minute, RefreshTTL: time.Hour, BcryptCost: 4},
	)

	for i := 0; i < 2; i++ {
		_, _ = svc.Login(context.Background(), auth.LoginRequest{Username: "alice", Password: "WRONG"}, "1.1.1.1", "ua")
	}
	_, err := svc.Login(context.Background(), auth.LoginRequest{Username: "alice", Password: "s3cret"}, "1.1.1.1", "ua")
	require.Error(t, err)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeRateLimited, be.Code)
}
