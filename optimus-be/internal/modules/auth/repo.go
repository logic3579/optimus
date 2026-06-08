package auth

import (
	"context"
	"time"

	"gorm.io/gorm"

	"optimus-be/internal/models"
)

type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) CreateRefreshToken(ctx context.Context, userID uint64, hash string, expiresAt time.Time, ua, ip string) (*models.RefreshToken, error) {
	rt := &models.RefreshToken{
		UserID:    userID,
		TokenHash: hash,
		ExpiresAt: expiresAt,
		UserAgent: ua,
		IP:        ip,
	}
	if err := r.db.WithContext(ctx).Create(rt).Error; err != nil {
		return nil, err
	}
	return rt, nil
}

func (r *Repo) FindRefreshTokenByHash(ctx context.Context, hash string) (*models.RefreshToken, error) {
	var rt models.RefreshToken
	if err := r.db.WithContext(ctx).Where("token_hash = ?", hash).First(&rt).Error; err != nil {
		return nil, err
	}
	return &rt, nil
}

func (r *Repo) RevokeRefreshToken(ctx context.Context, id uint64) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&models.RefreshToken{}).
		Where("id = ?", id).
		Update("revoked_at", &now).Error
}

func (r *Repo) RevokeAllRefreshTokensForUser(ctx context.Context, userID uint64) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&models.RefreshToken{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Update("revoked_at", &now).Error
}

// FindUserByUsername loads the user by username (only non-deleted, status='enabled').
func (r *Repo) FindUserByUsername(ctx context.Context, username string) (*models.User, error) {
	var u models.User
	if err := r.db.WithContext(ctx).Where("username = ? AND status = ?", username, "enabled").First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// UpdateLastLogin updates last_login_at to the given time.
func (r *Repo) UpdateLastLogin(ctx context.Context, userID uint64, at time.Time) error {
	return r.db.WithContext(ctx).
		Model(&models.User{}).
		Where("id = ?", userID).
		Update("last_login_at", &at).Error
}

// InsertAuditLog writes an audit row inline (no separate audit module yet).
func (r *Repo) InsertAuditLog(ctx context.Context, userID *uint64, action, ip, ua string, payload []byte) error {
	if payload == nil {
		payload = []byte("{}")
	}
	return r.db.WithContext(ctx).Create(&models.AuditLog{
		UserID:    userID,
		Action:    action,
		IP:        ip,
		UserAgent: ua,
		Payload:   payload,
	}).Error
}

// WithTx returns a Repo bound to the given transaction. The returned Repo can be
// used inside a Transaction callback to keep multiple operations atomic.
func (r *Repo) WithTx(tx *gorm.DB) *Repo { return &Repo{db: tx} }

// DB returns the underlying *gorm.DB. Use only when you need to start a Transaction.
func (r *Repo) DB() *gorm.DB { return r.db }
