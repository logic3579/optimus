package user

import "time"

type Summary struct {
	ID          uint64     `json:"id"`
	Username    string     `json:"username"`
	Email       string     `json:"email"`
	DisplayName string     `json:"display_name"`
	Status      string     `json:"status"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type Detail struct {
	Summary
	AvatarURL string    `json:"avatar_url"`
	Roles     []RoleRef `json:"roles"`
}

type RoleRef struct {
	ID   uint64 `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

type CreateRequest struct {
	Username    string   `json:"username"     binding:"required,min=3,max=64"`
	Email       string   `json:"email"        binding:"required,email,max=128"`
	Password    string   `json:"password"     binding:"required,min=8,max=128"`
	DisplayName string   `json:"display_name" binding:"max=128"`
	RoleIDs     []uint64 `json:"role_ids"`
}

type UpdateRequest struct {
	Email       *string `json:"email"        binding:"omitempty,email,max=128"`
	DisplayName *string `json:"display_name" binding:"omitempty,max=128"`
	AvatarURL   *string `json:"avatar_url"   binding:"omitempty,max=512"`
}

type SetRolesRequest struct {
	RoleIDs []uint64 `json:"role_ids" binding:"required"`
}

type SetStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=enabled disabled"`
}

type SetPasswordRequest struct {
	Password string `json:"password" binding:"required,min=8,max=128"`
}

type ListQuery struct {
	Search string // username/email LIKE
	Status string // "" | enabled | disabled
}
