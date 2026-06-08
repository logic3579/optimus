package rbac

import "time"

type MeUserDTO struct {
	ID          uint64     `json:"id"`
	Username    string     `json:"username"`
	Email       string     `json:"email"`
	DisplayName string     `json:"display_name"`
	AvatarURL   string     `json:"avatar_url"`
	Status      string     `json:"status"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
}

type MeMenuNode struct {
	ID             uint64       `json:"id"`
	Code           string       `json:"code"`
	Name           string       `json:"name"`
	Path           string       `json:"path"`
	Component      string       `json:"component"`
	Icon           string       `json:"icon"`
	PermissionCode *string      `json:"permission_code,omitempty"`
	SortOrder      int          `json:"sort_order"`
	Hidden         bool         `json:"hidden"`
	Children       []MeMenuNode `json:"children,omitempty"`
}

// UpdateMeRequest is the body for PUT /me.
type UpdateMeRequest struct {
	Email       *string `json:"email"        binding:"omitempty,email,max=128"`
	DisplayName *string `json:"display_name" binding:"omitempty,max=128"`
	AvatarURL   *string `json:"avatar_url"   binding:"omitempty,max=512"`
}

// ChangePasswordRequest is the body for PUT /me/password.
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required,min=1,max=128"`
	NewPassword string `json:"new_password" binding:"required,min=8,max=128"`
}
