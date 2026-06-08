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

func mkSvcStd(t *testing.T, gdb *gorm.DB) *auth.Service {
	return auth.NewService(
		auth.NewRepo(gdb),
		crypto.NewJWTSigner("test_secret_must_be_at_least_32_bytes_!!"),
		ratelimit.NewLoginLimiter(5, time.Minute, time.Minute),
		auth.ServiceOptions{AccessTTL: 5 * time.Minute, RefreshTTL: time.Hour, BcryptCost: 4},
	)
}

func loginAlice(t *testing.T, svc *auth.Service) *auth.TokenPair {
	pair, err := svc.Login(context.Background(), auth.LoginRequest{Username: "alice", Password: "s3cret"}, "1.1.1.1", "ua")
	require.NoError(t, err)
	return pair
}

func TestRefresh_RotatesTokens(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	seedUserWithPassword(t, gdb, "alice", "s3cret")
	svc := mkSvcStd(t, gdb)

	pair := loginAlice(t, svc)
	newPair, err := svc.Refresh(context.Background(), pair.RefreshToken, "1.1.1.1", "ua")
	require.NoError(t, err)

	require.NotEqual(t, pair.RefreshToken, newPair.RefreshToken)
	require.NotEqual(t, pair.AccessToken, newPair.AccessToken)

	var old models.RefreshToken
	gdb.Where("token_hash = ?", auth.Sha256HexForTest(pair.RefreshToken)).First(&old)
	require.NotNil(t, old.RevokedAt)
}

func TestRefresh_ReplayRevokesAllTokensAndAudits(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	seedUserWithPassword(t, gdb, "alice", "s3cret")
	svc := mkSvcStd(t, gdb)

	pair := loginAlice(t, svc)
	_, err := svc.Refresh(context.Background(), pair.RefreshToken, "1.1.1.1", "ua")
	require.NoError(t, err)

	_, err = svc.Refresh(context.Background(), pair.RefreshToken, "1.1.1.1", "ua")
	require.Error(t, err)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeRefreshTokenReplay, be.Code)

	var active int64
	gdb.Model(&models.RefreshToken{}).Where("revoked_at IS NULL").Count(&active)
	require.Equal(t, int64(0), active)

	var auditCount int64
	gdb.Model(&models.AuditLog{}).Where("action = ?", "auth.refresh.replay").Count(&auditCount)
	require.GreaterOrEqual(t, auditCount, int64(1))
}

func TestRefresh_RejectsExpiredToken(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	seedUserWithPassword(t, gdb, "alice", "s3cret")

	svc := auth.NewService(
		auth.NewRepo(gdb),
		crypto.NewJWTSigner("test_secret_must_be_at_least_32_bytes_!!"),
		ratelimit.NewLoginLimiter(5, time.Minute, time.Minute),
		auth.ServiceOptions{AccessTTL: 5 * time.Minute, RefreshTTL: -1 * time.Second, BcryptCost: 4},
	)
	pair, err := svc.Login(context.Background(), auth.LoginRequest{Username: "alice", Password: "s3cret"}, "1.1.1.1", "ua")
	require.NoError(t, err)

	_, err = svc.Refresh(context.Background(), pair.RefreshToken, "1.1.1.1", "ua")
	require.Error(t, err)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeTokenExpired, be.Code)
}

func TestRefresh_RejectsUnknownToken(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	svc := mkSvcStd(t, gdb)
	_, err := svc.Refresh(context.Background(), "not-a-real-token", "1.1.1.1", "ua")
	require.Error(t, err)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeTokenInvalid, be.Code)
}

var _ = gorm.DB{} // keep import
