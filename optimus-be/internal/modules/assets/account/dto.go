package account

import "time"

type Summary struct {
	ID             uint64     `json:"id"`
	Name           string     `json:"name"`
	Provider       string     `json:"provider"`
	CloudKeyID     uint64     `json:"cloudkey_id"`
	CloudKeyName   string     `json:"cloudkey_name"`
	RegionsCount   int        `json:"regions_count"`
	Enabled        bool       `json:"enabled"`
	LastSyncAt     *time.Time `json:"last_sync_at,omitempty"`
	LastSyncStatus string     `json:"last_sync_status,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type Detail struct {
	Summary
	EnabledRegions []string `json:"enabled_regions"`
	Description    string   `json:"description"`
}

type CreateRequest struct {
	Name           string   `json:"name"            binding:"required,min=1,max=128"`
	Provider       string   `json:"provider"        binding:"required,oneof=aws"`
	CloudKeyID     uint64   `json:"cloudkey_id"     binding:"required"`
	EnabledRegions []string `json:"enabled_regions" binding:"required,min=1,dive,min=1"`
	Enabled        *bool    `json:"enabled"`
	Description    string   `json:"description"     binding:"max=2000"`
}

type UpdateRequest struct {
	Name           *string  `json:"name,omitempty"            binding:"omitempty,min=1,max=128"`
	EnabledRegions []string `json:"enabled_regions,omitempty" binding:"omitempty,dive,min=1"`
	Enabled        *bool    `json:"enabled,omitempty"`
	Description    *string  `json:"description,omitempty"     binding:"omitempty,max=2000"`
}

type ListQuery struct {
	Q              string `form:"q"`
	Provider       string `form:"provider"`
	Enabled        *bool  `form:"enabled"`
	IncludeDeleted bool   `form:"include_deleted"`
	Page           int    `form:"page,default=1"  binding:"min=1"`
	Size           int    `form:"size,default=20" binding:"min=1,max=200"`
}

type ListResponse struct {
	Items []Summary `json:"items"`
	Total int64     `json:"total"`
}
