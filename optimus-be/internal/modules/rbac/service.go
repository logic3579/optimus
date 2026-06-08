package rbac

import (
	"context"

	"gorm.io/gorm"

	"optimus-be/internal/models"
)

type MeService struct {
	db    *gorm.DB
	cache *PermissionCache
}

func NewMeService(db *gorm.DB, cache *PermissionCache) *MeService {
	return &MeService{db: db, cache: cache}
}

func (s *MeService) GetUser(ctx context.Context, userID uint64) (*MeUserDTO, error) {
	var u models.User
	if err := s.db.WithContext(ctx).First(&u, userID).Error; err != nil {
		return nil, err
	}
	return &MeUserDTO{
		ID:          u.ID,
		Username:    u.Username,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		AvatarURL:   u.AvatarURL,
		Status:      u.Status,
		LastLoginAt: u.LastLoginAt,
	}, nil
}
