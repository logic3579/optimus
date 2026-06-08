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

// ListPermissions returns the permission codes the user has (via cache).
func (s *MeService) ListPermissions(ctx context.Context, userID uint64) ([]string, error) {
	return s.cache.Get(ctx, userID)
}

// ListMenus returns the menu tree filtered by the user's permissions.
// A node is included iff (permission_code is empty OR user has the code).
func (s *MeService) ListMenus(ctx context.Context, userID uint64) ([]MeMenuNode, error) {
	codes, err := s.cache.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	set := map[string]struct{}{}
	for _, c := range codes {
		set[c] = struct{}{}
	}

	var rows []models.Menu
	if err := s.db.WithContext(ctx).
		Where("hidden = ?", false).
		Order("sort_order ASC, id ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}

	byParent := map[uint64][]models.Menu{}
	const rootKey uint64 = 0
	for _, m := range rows {
		key := rootKey
		if m.ParentID != nil {
			key = *m.ParentID
		}
		byParent[key] = append(byParent[key], m)
	}

	var build func(parentID uint64) []MeMenuNode
	build = func(parentID uint64) []MeMenuNode {
		kids := byParent[parentID]
		out := make([]MeMenuNode, 0, len(kids))
		for _, m := range kids {
			if m.PermissionCode != nil {
				if _, ok := set[*m.PermissionCode]; !ok {
					continue
				}
			}
			node := MeMenuNode{
				ID:             m.ID,
				Code:           m.Code,
				Name:           m.Name,
				Path:           m.Path,
				Component:      m.Component,
				Icon:           m.Icon,
				PermissionCode: m.PermissionCode,
				SortOrder:      m.SortOrder,
				Hidden:         m.Hidden,
				Children:       build(m.ID),
			}
			out = append(out, node)
		}
		return out
	}
	return build(rootKey), nil
}
