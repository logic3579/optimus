package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"

	"optimus-be/internal/infra/crypto"
	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/ratelimit"
)

type ServiceOptions struct {
	AccessTTL  time.Duration
	RefreshTTL time.Duration
	BcryptCost int
}

type Service struct {
	repo    *Repo
	signer  *crypto.JWTSigner
	limiter *ratelimit.LoginLimiter
	opts    ServiceOptions
}

func NewService(repo *Repo, signer *crypto.JWTSigner, limiter *ratelimit.LoginLimiter, opts ServiceOptions) *Service {
	return &Service{repo: repo, signer: signer, limiter: limiter, opts: opts}
}

func (s *Service) Login(ctx context.Context, req LoginRequest, ip, ua string) (*TokenPair, error) {
	if !s.limiter.Allow(ip, req.Username) {
		_ = s.repo.InsertAuditLog(ctx, nil, "auth.login.rate_limited", ip, ua, mustJSON(map[string]any{"username": req.Username}))
		return nil, apperr.New(apperr.CodeRateLimited, "auth.rate_limited", "too many login attempts")
	}

	u, err := s.repo.FindUserByUsername(ctx, req.Username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			_ = s.repo.InsertAuditLog(ctx, nil, "auth.login.failed", ip, ua, mustJSON(map[string]any{"username": req.Username, "reason": "not_found"}))
			return nil, apperr.New(apperr.CodeInvalidCredentials, "auth.invalid_credentials", "invalid username or password")
		}
		return nil, err
	}
	if err := crypto.ComparePassword(u.PasswordHash, req.Password); err != nil {
		uid := u.ID
		_ = s.repo.InsertAuditLog(ctx, &uid, "auth.login.failed", ip, ua, mustJSON(map[string]any{"reason": "bad_password"}))
		return nil, apperr.New(apperr.CodeInvalidCredentials, "auth.invalid_credentials", "invalid username or password")
	}

	pair, err := s.issuePair(ctx, u.ID, ip, ua)
	if err != nil {
		return nil, err
	}
	_ = s.repo.UpdateLastLogin(ctx, u.ID, time.Now())
	uid := u.ID
	_ = s.repo.InsertAuditLog(ctx, &uid, "auth.login.success", ip, ua, nil)
	return pair, nil
}

func (s *Service) issuePair(ctx context.Context, userID uint64, ip, ua string) (*TokenPair, error) {
	jti, err := randomHex(16)
	if err != nil {
		return nil, err
	}
	access, err := s.signer.Sign(crypto.JWTClaims{UserID: userID, JTI: jti}, s.opts.AccessTTL)
	if err != nil {
		return nil, err
	}
	refresh, err := randomBase64(32)
	if err != nil {
		return nil, err
	}
	hash := sha256Hex(refresh)
	expiresAt := time.Now().Add(s.opts.RefreshTTL)
	if _, err := s.repo.CreateRefreshToken(ctx, userID, hash, expiresAt, ua, ip); err != nil {
		return nil, err
	}
	return &TokenPair{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresAt:    time.Now().Add(s.opts.AccessTTL),
	}, nil
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// Sha256HexForTest is exported for test files in this package to compute the same
// hash the service stores for refresh tokens.
func Sha256HexForTest(s string) string { return sha256Hex(s) }

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func randomBase64(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// Refresh validates the refresh token, rotates the pair, and detects replay.
// If the supplied refresh token is already revoked, ALL of the user's refresh
// tokens are revoked and an audit row is written.
func (s *Service) Refresh(ctx context.Context, refresh, ip, ua string) (*TokenPair, error) {
	hash := sha256Hex(refresh)
	row, err := s.repo.FindRefreshTokenByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.New(apperr.CodeTokenInvalid, "auth.token_invalid", "refresh token not recognized")
		}
		return nil, err
	}
	if row.RevokedAt != nil {
		_ = s.repo.RevokeAllRefreshTokensForUser(ctx, row.UserID)
		uid := row.UserID
		_ = s.repo.InsertAuditLog(ctx, &uid, "auth.refresh.replay", ip, ua, nil)
		return nil, apperr.New(apperr.CodeRefreshTokenReplay, "auth.refresh_replay", "refresh token replay detected")
	}
	if time.Now().After(row.ExpiresAt) {
		return nil, apperr.New(apperr.CodeTokenExpired, "auth.token_expired", "refresh token expired")
	}

	// Atomic rotation: revoke old + issue new in one transaction.
	var pair *TokenPair
	if err := s.repo.DB().Transaction(func(tx *gorm.DB) error {
		txRepo := s.repo.WithTx(tx)
		if err := txRepo.RevokeRefreshToken(ctx, row.ID); err != nil {
			return err
		}
		// inline pair issuance using tx-bound repo
		jti, err := randomHex(16)
		if err != nil {
			return err
		}
		access, err := s.signer.Sign(crypto.JWTClaims{UserID: row.UserID, JTI: jti}, s.opts.AccessTTL)
		if err != nil {
			return err
		}
		refreshNew, err := randomBase64(32)
		if err != nil {
			return err
		}
		expiresAt := time.Now().Add(s.opts.RefreshTTL)
		if _, err := txRepo.CreateRefreshToken(ctx, row.UserID, sha256Hex(refreshNew), expiresAt, ua, ip); err != nil {
			return err
		}
		pair = &TokenPair{
			AccessToken:  access,
			RefreshToken: refreshNew,
			ExpiresAt:    time.Now().Add(s.opts.AccessTTL),
		}
		return nil
	}); err != nil {
		return nil, err
	}

	// Audit refresh success (write outside the tx so an audit-write failure
	// doesn't undo a successful rotation).
	uid := row.UserID
	_ = s.repo.InsertAuditLog(ctx, &uid, "auth.refresh.success", ip, ua, nil)

	return pair, nil
}

// Logout revokes the given refresh token. Idempotent: unknown / already-revoked tokens
// are not errors.
func (s *Service) Logout(ctx context.Context, refresh string) error {
	if refresh == "" {
		return nil
	}
	row, err := s.repo.FindRefreshTokenByHash(ctx, sha256Hex(refresh))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if row.RevokedAt != nil {
		return nil
	}
	if err := s.repo.RevokeRefreshToken(ctx, row.ID); err != nil {
		return err
	}
	uid := row.UserID
	_ = s.repo.InsertAuditLog(ctx, &uid, "auth.logout", "", "", nil)
	return nil
}
