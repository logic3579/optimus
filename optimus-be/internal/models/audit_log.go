package models

import (
	"time"

	"gorm.io/datatypes"
)

type AuditLog struct {
	ID         uint64         `gorm:"primaryKey"`
	UserID     *uint64        `gorm:"index"`
	Action     string         `gorm:"size:64;not null;index"`
	TargetType string         `gorm:"size:64;not null;default:''"`
	TargetID   string         `gorm:"size:64;not null;default:''"`
	Payload    datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'::jsonb"`
	IP         string         `gorm:"size:64;not null;default:''"`
	UserAgent  string         `gorm:"size:512;not null;default:''"`
	CreatedAt  time.Time      `gorm:"index"`
}

func (AuditLog) TableName() string { return "audit_logs" }
