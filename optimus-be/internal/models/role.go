package models

import (
	"time"

	"gorm.io/gorm"
)

type Role struct {
	ID          uint64 `gorm:"primaryKey"`
	Code        string `gorm:"size:64;not null"`
	Name        string `gorm:"size:128;not null"`
	Description string `gorm:"size:512;not null;default:''"`
	IsBuiltin   bool   `gorm:"not null;default:false"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

func (Role) TableName() string { return "roles" }
