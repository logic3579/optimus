package kubeconfig

import "time"

type Summary struct {
	ID               uint64    `json:"id"`
	Name             string    `json:"name"`
	Description      string    `json:"description"`
	DefaultNamespace string    `json:"default_namespace"`
	CreatedBy        *Actor    `json:"created_by,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type Detail = Summary

type Actor struct {
	ID          uint64 `json:"id"`
	Username    string `json:"username,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
}

type CreateRequest struct {
	Name             string `json:"name"              binding:"required,max=128"`
	Description      string `json:"description"       binding:"max=4096"`
	DefaultNamespace string `json:"default_namespace" binding:"max=64"`
	Kubeconfig       string `json:"kubeconfig"        binding:"required"`
}

type UpdateRequest struct {
	Name             *string `json:"name,omitempty"              binding:"omitempty,max=128"`
	Description      *string `json:"description,omitempty"       binding:"omitempty,max=4096"`
	DefaultNamespace *string `json:"default_namespace,omitempty" binding:"omitempty,max=64"`
	Kubeconfig       *string `json:"kubeconfig,omitempty"`
}

type ListQuery struct {
	Page             int    `form:"page,default=1"`
	PageSize         int    `form:"page_size,default=20"`
	Q                string `form:"q"`
	DefaultNamespace string `form:"default_namespace"`
}

type ListResponse struct {
	Items    []Summary `json:"items"`
	Total    int64     `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
}
