package role

import "time"

type Summary struct {
	ID          uint64    `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsBuiltin   bool      `json:"is_builtin"`
	CreatedAt   time.Time `json:"created_at"`
}

type Detail struct {
	Summary
	PermissionCodes []string `json:"permission_codes"`
}

type CreateRequest struct {
	Code        string `json:"code"        binding:"required,min=2,max=64"`
	Name        string `json:"name"        binding:"required,max=128"`
	Description string `json:"description" binding:"max=512"`
}

type UpdateRequest struct {
	Name        *string `json:"name"        binding:"omitempty,max=128"`
	Description *string `json:"description" binding:"omitempty,max=512"`
}

type SetPermissionsRequest struct {
	PermissionCodes []string `json:"permission_codes" binding:"required"`
}
