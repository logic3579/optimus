package repo

import "time"

// Summary is the list-row shape; never includes encrypted_password.
type Summary struct {
	ID          uint64    `json:"id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	URL         string    `json:"url"`
	Username    string    `json:"username"`
	HasPassword bool      `json:"has_password"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Detail equals Summary for chart repos.
type Detail = Summary

// CreateRequest is the JSON payload for POST /apps/repos.
type CreateRequest struct {
	Name        string `json:"name"        binding:"required,max=64"`
	Type        string `json:"type"        binding:"required,oneof=oci http"`
	URL         string `json:"url"         binding:"required,max=2048"`
	Username    string `json:"username"    binding:"max=255"`
	Password    string `json:"password"`
	Description string `json:"description" binding:"max=4096"`
}

// UpdateRequest password semantics:
//   - field absent / empty string -> keep current encrypted_password.
//   - field explicit null         -> clear encrypted_password (handler stuffs a sentinel).
//
// type is silently ignored.
type UpdateRequest struct {
	Name        *string `json:"name,omitempty"        binding:"omitempty,max=64"`
	URL         *string `json:"url,omitempty"         binding:"omitempty,max=2048"`
	Username    *string `json:"username,omitempty"    binding:"omitempty,max=255"`
	Password    *string `json:"password,omitempty"`
	Description *string `json:"description,omitempty" binding:"omitempty,max=4096"`
}

// ListQuery is the query-string shape for GET /apps/repos.
type ListQuery struct {
	Page     int    `form:"page,default=1"`
	PageSize int    `form:"page_size,default=20"`
	Name     string `form:"name"`
	Type     string `form:"type"`
}

// ListResponse is the paged result envelope for GET /apps/repos.
type ListResponse struct {
	Items    []Summary `json:"items"`
	Total    int64     `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
}

// ChartSummary is one chart's name + the count of versions in the upstream index.
type ChartSummary struct {
	Name         string `json:"name"`
	VersionCount int    `json:"version_count"`
	Description  string `json:"description"` // best-available description from index
}

// VersionSummary is a single chart version row.
type VersionSummary struct {
	Version    string `json:"version"`
	AppVersion string `json:"app_version"`
	Created    string `json:"created"`
}
