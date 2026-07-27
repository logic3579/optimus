package project

import "time"

// Application is the safe application projection delivery needs. The apps
// adapter is responsible for determining whether the Helm release is installed.
type Application struct {
	ID          uint64 `json:"id"`
	Name        string `json:"name"`
	ChartRepoID uint64 `json:"chart_repo_id"`
	ChartName   string `json:"chart_name"`
	Installed   bool   `json:"installed"`
	ClusterID   uint64 `json:"cluster_id"`
	Namespace   string `json:"namespace"`
	ReleaseName string `json:"release_name"`
}

// Environment is a delivery binding enriched with its safe application data.
type Environment struct {
	ID              uint64    `json:"id"`
	ProjectID       uint64    `json:"project_id"`
	EnvironmentKey  string    `json:"environment_key"`
	DisplayName     string    `json:"display_name"`
	ApplicationID   uint64    `json:"application_id"`
	ApplicationName string    `json:"application_name"`
	ChartRepoID     uint64    `json:"chart_repo_id"`
	ChartName       string    `json:"chart_name"`
	Installed       bool      `json:"installed"`
	ClusterID       uint64    `json:"cluster_id"`
	Namespace       string    `json:"namespace"`
	ReleaseName     string    `json:"release_name"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// ProjectSummary is the project list-row shape.
type ProjectSummary struct {
	ID               uint64    `json:"id"`
	Name             string    `json:"name"`
	Description      string    `json:"description"`
	OwnerUserID      *uint64   `json:"owner_user_id,omitempty"`
	EnvironmentCount int64     `json:"environment_count"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// ProjectDetail includes active environments in stable key order.
type ProjectDetail struct {
	ProjectSummary
	Environments []Environment `json:"environments"`
}

type ListQuery struct {
	Page     int    `form:"page,default=1" binding:"min=1"`
	PageSize int    `form:"page_size,default=20" binding:"min=1,max=100"`
	Q        string `form:"q" binding:"max=128"`
}

type ListResponse struct {
	Items    []ProjectSummary `json:"items"`
	Total    int64            `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
}

type CreateProjectRequest struct {
	Name        string  `json:"name" binding:"required,min=1,max=128"`
	Description string  `json:"description" binding:"max=4096"`
	OwnerUserID *uint64 `json:"owner_user_id,omitempty" binding:"omitempty,min=1"`
}

type UpdateProjectRequest struct {
	Name        *string `json:"name,omitempty" binding:"omitempty,min=1,max=128"`
	Description *string `json:"description,omitempty" binding:"omitempty,max=4096"`
	// OwnerUserID uses zero to clear the nullable owner and a non-zero value to set it.
	OwnerUserID *uint64 `json:"owner_user_id,omitempty"`
}

type BindEnvironmentRequest struct {
	EnvironmentKey string `json:"environment_key" binding:"required,min=1,max=128"`
	DisplayName    string `json:"display_name" binding:"required,min=1,max=128"`
	ApplicationID  uint64 `json:"application_id" binding:"required"`
}

type UpdateEnvironmentRequest struct {
	EnvironmentKey *string `json:"environment_key,omitempty" binding:"omitempty,min=1,max=128"`
	DisplayName    *string `json:"display_name,omitempty" binding:"omitempty,min=1,max=128"`
	// ApplicationID is accepted only to return an explicit error for attempted
	// rebinding. Application bindings are immutable in the MVP.
	ApplicationID *uint64 `json:"application_id,omitempty" binding:"omitempty,min=1"`
}
