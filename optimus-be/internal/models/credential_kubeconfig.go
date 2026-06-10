package models

import "time"

type CredentialKubeconfig struct {
	ID               uint64  `gorm:"primaryKey"`
	Name             string  `gorm:"size:128;not null;uniqueIndex"`
	Description      string  `gorm:"type:text;not null;default:''"`
	DefaultNamespace string  `gorm:"column:default_namespace;size:64;not null;default:''"`
	KubeconfigEnc    []byte  `gorm:"column:kubeconfig_enc;not null"`
	CreatedByUserID  *uint64 `gorm:"column:created_by_user_id"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (CredentialKubeconfig) TableName() string { return "credentials_kubeconfigs" }
