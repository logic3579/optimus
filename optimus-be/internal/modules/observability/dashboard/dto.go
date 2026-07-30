package dashboard

import "time"

var Units = map[string]struct{}{"none": {}, "percent": {}, "bytes": {}, "bytes_per_second": {}, "cores": {}, "seconds": {}, "requests_per_second": {}}

type PanelInput struct {
	DatasourceID uint64 `json:"datasource_id" binding:"required"`
	Title        string `json:"title" binding:"required,max=128"`
	PanelType    string `json:"panel_type" binding:"required"`
	PromQL       string `json:"promql" binding:"required,max=8192"`
	Unit         string `json:"unit" binding:"required"`
	Legend       string `json:"legend" binding:"max=128"`
	SortOrder    int    `json:"sort_order"`
	Width        int    `json:"width" binding:"required"`
}
type SaveRequest struct {
	Name             string       `json:"name" binding:"required,max=128"`
	Description      string       `json:"description" binding:"max=4096"`
	RefreshIntervalS int          `json:"refresh_interval_s" binding:"required"`
	TimeRange        string       `json:"time_range" binding:"required"`
	Panels           []PanelInput `json:"panels" binding:"required,dive"`
}
type Panel struct {
	ID           uint64    `json:"id"`
	DashboardID  uint64    `json:"dashboard_id"`
	DatasourceID uint64    `json:"datasource_id"`
	Title        string    `json:"title"`
	PanelType    string    `json:"panel_type"`
	PromQL       string    `json:"promql"`
	Unit         string    `json:"unit"`
	Legend       string    `json:"legend"`
	SortOrder    int       `json:"sort_order"`
	Width        int       `json:"width"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
type Detail struct {
	ID               uint64    `json:"id"`
	Name             string    `json:"name"`
	Description      string    `json:"description"`
	RefreshIntervalS int       `json:"refresh_interval_s"`
	TimeRange        string    `json:"time_range"`
	CreatedByUserID  *uint64   `json:"created_by_user_id,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	Panels           []Panel   `json:"panels"`
}
type ListQuery struct {
	Page     int    `form:"page,default=1"`
	PageSize int    `form:"page_size,default=20"`
	Q        string `form:"q"`
}
type ListResponse struct {
	Items    []Detail `json:"items"`
	Total    int64    `json:"total"`
	Page     int      `json:"page"`
	PageSize int      `json:"page_size"`
}
