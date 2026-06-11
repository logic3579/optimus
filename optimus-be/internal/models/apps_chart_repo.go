package models

import (
	"time"

	"gorm.io/gorm"
)

// AppsChartRepo is a registered Helm chart source (OCI or HTTP).
// EncryptedPassword holds bytes from P1's vault.Cipher. The repo service is
// responsible for decryption at use time; plaintext is never persisted or
// held on the struct beyond a single function call.
type AppsChartRepo struct {
	ID                uint64 `gorm:"primaryKey"`
	Name              string `gorm:"size:64;not null"`
	Type              string `gorm:"size:8;not null"`
	URL               string `gorm:"type:text;not null"`
	Username          string `gorm:"size:255;not null;default:''"`
	EncryptedPassword []byte `gorm:"type:bytea;not null;default:'\\x'"`
	Description       string `gorm:"type:text;not null;default:''"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         gorm.DeletedAt `gorm:"index"`
}

func (AppsChartRepo) TableName() string { return "apps_chart_repos" }
