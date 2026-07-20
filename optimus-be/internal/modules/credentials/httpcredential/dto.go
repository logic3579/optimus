package httpcredential

import "time"

type Actor struct {
	ID          uint64 `json:"id"`
	Username    string `json:"username,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
}
type Summary struct {
	ID        uint64    `json:"id"`
	Name      string    `json:"name"`
	AuthType  string    `json:"auth_type"`
	Username  *string   `json:"username,omitempty"`
	CreatedBy *Actor    `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type Detail = Summary
type CreateRequest struct {
	Name     string  `json:"name" binding:"required,max=128"`
	AuthType string  `json:"auth_type" binding:"required,oneof=basic bearer"`
	Username *string `json:"username,omitempty" binding:"omitempty,max=256"`
	Secret   string  `json:"secret" binding:"required,max=16384"`
}
type UpdateRequest struct {
	Name     *string `json:"name,omitempty" binding:"omitempty,max=128"`
	Username *string `json:"username,omitempty" binding:"omitempty,max=256"`
	Secret   *string `json:"secret,omitempty" binding:"omitempty,max=16384"`
}
type ListQuery struct {
	Page     int    `form:"page,default=1"`
	PageSize int    `form:"page_size,default=20"`
	Q        string `form:"q"`
	AuthType string `form:"auth_type" binding:"omitempty,oneof=basic bearer"`
}
type ListResponse struct {
	Items    []Summary `json:"items"`
	Total    int64     `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
}
