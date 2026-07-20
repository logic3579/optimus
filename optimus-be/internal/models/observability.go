package models

import (
	"time"

	"gorm.io/gorm"
)

type ObservabilityDatasource struct {
	ID                                   uint64 `gorm:"primaryKey"`
	Name, BaseURL, AuthType, Description string
	HTTPCredentialID, ClusterID          *uint64
	TLSSkipVerify                        bool
	CustomCAPEM                          *string `gorm:"column:custom_ca_pem"`
	CreatedByUserID                      *uint64
	CreatedAt, UpdatedAt                 time.Time
	DeletedAt                            gorm.DeletedAt `gorm:"index"`
}

func (ObservabilityDatasource) TableName() string { return "observability_datasources" }

type ObservabilityDashboard struct {
	ID                           uint64 `gorm:"primaryKey"`
	Name, Description, TimeRange string
	RefreshIntervalS             int
	CreatedByUserID              *uint64
	CreatedAt, UpdatedAt         time.Time
	DeletedAt                    gorm.DeletedAt `gorm:"index"`
}

func (ObservabilityDashboard) TableName() string { return "observability_dashboards" }

type ObservabilityPanel struct {
	ID, DashboardID, DatasourceID uint64
	Title, PanelType              string
	PromQL                        string `gorm:"column:promql"`
	Unit, Legend                  string
	SortOrder, Width              int
	CreatedAt, UpdatedAt          time.Time
}

func (ObservabilityPanel) TableName() string { return "observability_panels" }
