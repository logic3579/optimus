package sshkey

import "time"

// Summary is the list-row shape. Never contains secret material.
type Summary struct {
	ID          uint64    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Username    string    `json:"username"`
	CreatedBy   *Actor    `json:"created_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Detail is the get-by-id shape. Identical to Summary for SSH keys.
type Detail = Summary

// Actor is the populated creator. ID-only when the user is deleted.
type Actor struct {
	ID          uint64 `json:"id"`
	Username    string `json:"username,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
}

type CreateRequest struct {
	Name        string `json:"name"        binding:"required,max=128"`
	Description string `json:"description" binding:"max=4096"`
	Username    string `json:"username"    binding:"required,max=64"`
	PrivateKey  string `json:"private_key" binding:"required"`
	Passphrase  string `json:"passphrase"`
}

type UpdateRequest struct {
	Name        *string `json:"name,omitempty"        binding:"omitempty,max=128"`
	Description *string `json:"description,omitempty" binding:"omitempty,max=4096"`
	Username    *string `json:"username,omitempty"    binding:"omitempty,max=64"`
	PrivateKey  *string `json:"private_key,omitempty"`
	Passphrase  *string `json:"passphrase,omitempty"`
}

type ListQuery struct {
	Page     int    `form:"page,default=1"`
	PageSize int    `form:"page_size,default=20"`
	Q        string `form:"q"`
	Username string `form:"username"`
}

type ListResponse struct {
	Items    []Summary `json:"items"`
	Total    int64     `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
}
