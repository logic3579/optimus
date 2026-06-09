package rbac

import (
	"context"

	"gorm.io/gorm"

	"optimus-be/internal/models"
)

// UserWriter is the seam MeService uses to mutate user records on behalf of
// the authenticated principal. It exists as an interface (rather than a
// direct *user.Service dependency) because user.Service already imports
// rbac.PermissionCache — a direct import would create a cycle.
//
// user.Service satisfies this interface implicitly via the adapter methods
// UpdateProfile and ChangePassword (see internal/modules/user).
type UserWriter interface {
	UpdateProfile(ctx context.Context, actorID uint64, ip, ua string, id uint64, email, displayName, avatarURL *string) error
	ChangePassword(ctx context.Context, userID uint64, ip, ua, oldPassword, newPassword string) error
}

type MeService struct {
	db    *gorm.DB
	cache *PermissionCache
	users UserWriter
}

func NewMeService(db *gorm.DB, cache *PermissionCache, users UserWriter) *MeService {
	return &MeService{db: db, cache: cache, users: users}
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

// UpdateMe applies partial profile edits on behalf of the authenticated user
// and returns the refreshed Me DTO. The actual mutation + audit + uniqueness
// checks are delegated to user.Service via the UserWriter seam so the /me
// write path reuses the same rules as admin /users edits.
func (s *MeService) UpdateMe(ctx context.Context, userID uint64, ip, ua string, req UpdateMeRequest) (*MeUserDTO, error) {
	if err := s.users.UpdateProfile(ctx, userID, ip, ua, userID, req.Email, req.DisplayName, req.AvatarURL); err != nil {
		return nil, err
	}
	return s.GetUser(ctx, userID)
}

// ChangeMyPassword verifies the user's old password and rotates the hash.
// Forwards directly to user.Service.ChangePassword so the bcrypt cost +
// audit semantics stay centralised there.
func (s *MeService) ChangeMyPassword(ctx context.Context, userID uint64, ip, ua, oldPassword, newPassword string) error {
	return s.users.ChangePassword(ctx, userID, ip, ua, oldPassword, newPassword)
}
