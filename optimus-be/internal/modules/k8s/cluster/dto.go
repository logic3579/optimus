package cluster

import "time"

type Summary struct {
	ID             uint64     `json:"id"`
	Name           string     `json:"name"`
	KubeconfigID   uint64     `json:"kubeconfig_id"`
	KubeconfigName string     `json:"kubeconfig_name,omitempty"` // joined when listing / getting
	Context        string     `json:"context"`
	Description    string     `json:"description"`
	Tags           []string   `json:"tags"`
	LastHealthAt   *time.Time `json:"last_health_at,omitempty"`
	LastHealthOK   *bool      `json:"last_health_ok,omitempty"`
	LastHealthMsg  string     `json:"last_health_msg,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type Detail = Summary

type CreateRequest struct {
	Name         string   `json:"name"          binding:"required,max=64"`
	KubeconfigID uint64   `json:"kubeconfig_id" binding:"required"`
	Context      string   `json:"context"       binding:"required,max=128"`
	Description  string   `json:"description"   binding:"max=4096"`
	Tags         []string `json:"tags"          binding:"omitempty,dive,max=64"`
}

type UpdateRequest struct {
	Name         *string   `json:"name,omitempty"          binding:"omitempty,max=64"`
	KubeconfigID *uint64   `json:"kubeconfig_id,omitempty"`
	Context      *string   `json:"context,omitempty"       binding:"omitempty,max=128"`
	Description  *string   `json:"description,omitempty"   binding:"omitempty,max=4096"`
	Tags         *[]string `json:"tags,omitempty"          binding:"omitempty,dive,max=64"`
}

// ListQuery binds query-string parameters. `Tag` is validated at the handler
// to a [a-zA-Z0-9_-]+ charset before reaching the repo, because the repo's
// JSONB containment filter builds its operand via string concatenation.
type ListQuery struct {
	Page         int    `form:"page,default=1"`
	PageSize     int    `form:"page_size,default=20"`
	Search       string `form:"search"`
	Tag          string `form:"tag"`
	KubeconfigID uint64 `form:"kubeconfig_id"`
}

type ListResponse struct {
	Items    []Summary `json:"items"`
	Total    int64     `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
}

type PingResult struct {
	OK            bool   `json:"ok"`
	ServerVersion string `json:"server_version,omitempty"`
	Message       string `json:"message,omitempty"`
}
