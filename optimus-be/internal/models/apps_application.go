package models

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// AppsApplication is the Optimus row pointing at a Helm release on a
// specific cluster+namespace. (cluster_id, namespace, release_name) is
// unique per spec; chart_name is immutable post-creation; chart_repo_id
// is mutable only by the upgrade endpoint (NOT by PUT /applications/:id).
type AppsApplication struct {
	ID          uint64                      `gorm:"primaryKey"`
	Name        string                      `gorm:"size:64;not null"`
	ClusterID   uint64                      `gorm:"column:cluster_id;not null;index"`
	Namespace   string                      `gorm:"size:63;not null"`
	ReleaseName string                      `gorm:"column:release_name;size:53;not null"`
	ChartRepoID uint64                      `gorm:"column:chart_repo_id;not null"`
	ChartName   string                      `gorm:"column:chart_name;size:128;not null"`
	Description string                      `gorm:"type:text;not null;default:''"`
	Tags        datatypes.JSONSlice[string] `gorm:"type:jsonb;not null;default:'[]'"`
	OwnerUserID *uint64                     `gorm:"column:owner_user_id;index"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`

	// Preload-friendly associations. Populated on Get / List with .Preload.
	Cluster   *Cluster       `gorm:"foreignKey:ClusterID;references:ID"`
	ChartRepo *AppsChartRepo `gorm:"foreignKey:ChartRepoID;references:ID"`
	OwnerUser *User          `gorm:"foreignKey:OwnerUserID;references:ID"`
}

func (AppsApplication) TableName() string { return "apps_applications" }
