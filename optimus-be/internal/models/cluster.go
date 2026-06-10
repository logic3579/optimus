package models

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Cluster struct {
	ID            uint64                      `gorm:"primaryKey"`
	Name          string                      `gorm:"size:64;not null"`
	KubeconfigID  uint64                      `gorm:"column:kubeconfig_id;not null"`
	Context       string                      `gorm:"size:128;not null"`
	Description   string                      `gorm:"type:text;not null;default:''"`
	Tags          datatypes.JSONSlice[string] `gorm:"type:jsonb;not null;default:'[]'"`
	LastHealthAt  *time.Time                  `gorm:"column:last_health_at"`
	LastHealthOK  *bool                       `gorm:"column:last_health_ok"`
	LastHealthMsg string                      `gorm:"column:last_health_msg;type:text;not null;default:''"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     gorm.DeletedAt `gorm:"index"`

	// Optional preload — populated only when explicitly Preload("Kubeconfig").
	Kubeconfig *CredentialKubeconfig `gorm:"foreignKey:KubeconfigID;references:ID"`
}

func (Cluster) TableName() string { return "clusters" }
