package application

import "time"

// Summary is the list-row shape. Live helm status is null in list responses;
// FE may opt-in via per-row GET /release for the visible page.
type Summary struct {
	ID          uint64    `json:"id"`
	Name        string    `json:"name"`
	ClusterID   uint64    `json:"cluster_id"`
	ClusterName string    `json:"cluster_name"`
	Namespace   string    `json:"namespace"`
	ReleaseName string    `json:"release_name"`
	ChartRepoID uint64    `json:"chart_repo_id"`
	ChartName   string    `json:"chart_name"`
	Description string    `json:"description"`
	Tags        []string  `json:"tags"`
	OwnerUserID *uint64   `json:"owner_user_id,omitempty"`
	OwnerName   string    `json:"owner_name,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Detail extends Summary with live helm status fetched on demand.
type Detail struct {
	Summary
	Status         string `json:"status,omitempty"`
	Revision       *int   `json:"revision,omitempty"`
	ChartVersion   string `json:"chart_version,omitempty"`
	AppVersion     string `json:"app_version,omitempty"`
	LastDeployedAt string `json:"last_deployed_at,omitempty"`
}

// CreateRequest is the JSON payload for POST /apps/applications.
type CreateRequest struct {
	Name        string   `json:"name"          binding:"required,max=64"`
	ClusterID   uint64   `json:"cluster_id"    binding:"required"`
	Namespace   string   `json:"namespace"     binding:"required,max=63"`
	ReleaseName string   `json:"release_name"  binding:"required,max=53"`
	ChartRepoID uint64   `json:"chart_repo_id" binding:"required"`
	ChartName   string   `json:"chart_name"    binding:"required,max=128"`
	Description string   `json:"description"   binding:"max=4096"`
	Tags        []string `json:"tags"          binding:"omitempty,dive,max=32"`
	OwnerUserID *uint64  `json:"owner_user_id"`
}

// UpdateRequest only touches the three mutable metadata fields; chart_repo_id
// is mutated only through release.Upgrade via Service.SetChartRepo.
type UpdateRequest struct {
	Description *string  `json:"description,omitempty" binding:"omitempty,max=4096"`
	Tags        []string `json:"tags,omitempty"        binding:"omitempty,dive,max=32"`
	OwnerUserID *uint64  `json:"owner_user_id,omitempty"`
}

// ListQuery is the query-string shape for GET /apps/applications.
type ListQuery struct {
	Page        int    `form:"page,default=1"`
	PageSize    int    `form:"page_size,default=20"`
	Name        string `form:"name"`
	ClusterID   uint64 `form:"cluster_id"`
	Namespace   string `form:"namespace"`
	OwnerUserID uint64 `form:"owner_user_id"`
	Tag         string `form:"tag"`
}

// ListResponse is the paged result envelope.
type ListResponse struct {
	Items    []Summary `json:"items"`
	Total    int64     `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
}
