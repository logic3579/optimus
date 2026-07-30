package datasource

import "time"

type CreateRequest struct {
	Name             string  `json:"name" binding:"required,max=128"`
	BaseURL          string  `json:"base_url" binding:"required"`
	AuthType         string  `json:"auth_type" binding:"required,oneof=none basic bearer"`
	HTTPCredentialID *uint64 `json:"http_credential_id,omitempty"`
	ClusterID        *uint64 `json:"cluster_id,omitempty"`
	TLSSkipVerify    bool    `json:"tls_skip_verify"`
	CustomCAPEM      *string `json:"custom_ca_pem,omitempty"`
	Description      string  `json:"description" binding:"max=4096"`
}

type UpdateRequest struct {
	Name                *string `json:"name,omitempty" binding:"omitempty,max=128"`
	BaseURL             *string `json:"base_url,omitempty"`
	AuthType            *string `json:"auth_type,omitempty" binding:"omitempty,oneof=none basic bearer"`
	HTTPCredentialID    *uint64 `json:"http_credential_id,omitempty"`
	ClearHTTPCredential bool    `json:"clear_http_credential,omitempty"`
	ClusterID           *uint64 `json:"cluster_id,omitempty"`
	ClearCluster        bool    `json:"clear_cluster,omitempty"`
	TLSSkipVerify       *bool   `json:"tls_skip_verify,omitempty"`
	CustomCAPEM         *string `json:"custom_ca_pem,omitempty"`
	ClearCustomCA       bool    `json:"clear_custom_ca,omitempty"`
	Description         *string `json:"description,omitempty" binding:"omitempty,max=4096"`
}

type NamedRef struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
}
type Detail struct {
	ID              uint64    `json:"id"`
	Name            string    `json:"name"`
	BaseURL         string    `json:"base_url"`
	AuthType        string    `json:"auth_type"`
	HTTPCredential  *NamedRef `json:"http_credential,omitempty"`
	Cluster         *NamedRef `json:"cluster,omitempty"`
	TLSSkipVerify   bool      `json:"tls_skip_verify"`
	HasCustomCA     bool      `json:"has_custom_ca"`
	Description     string    `json:"description"`
	CreatedByUserID *uint64   `json:"created_by_user_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	customCAPEM     string
}

// CustomCAPEMCopy returns request-scoped TLS material without exposing it in
// the JSON data-source DTO or aliasing internal storage.
func (d Detail) CustomCAPEMCopy() []byte {
	if d.customCAPEM == "" {
		return nil
	}
	return []byte(d.customCAPEM)
}

type ListQuery struct {
	Page      int     `form:"page,default=1"`
	PageSize  int     `form:"page_size,default=20"`
	Q         string  `form:"q"`
	AuthType  string  `form:"auth_type"`
	ClusterID *uint64 `form:"cluster_id"`
}
type ListResponse struct {
	Items    []Detail `json:"items"`
	Total    int64    `json:"total"`
	Page     int      `json:"page"`
	PageSize int      `json:"page_size"`
}

// QuerySource is the minimal data-source identity exposed to metric operators.
// It intentionally excludes connection, authentication, and TLS metadata.
type QuerySource struct {
	ID        uint64  `json:"id"`
	Name      string  `json:"name"`
	ClusterID *uint64 `json:"cluster_id"`
}
type TestResponse struct {
	Reachable bool   `json:"reachable"`
	Version   string `json:"version,omitempty"`
}
