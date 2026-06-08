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
