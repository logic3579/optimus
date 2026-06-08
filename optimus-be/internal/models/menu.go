package models

import (
	"time"

	"gorm.io/gorm"
)

type Menu struct {
	ID             uint64         `gorm:"primaryKey"`
	ParentID       *uint64
	Code           string         `gorm:"size:64;not null"`
	Name           string         `gorm:"size:128;not null"`
	Path           string         `gorm:"size:255;not null;default:''"`
	Component      string         `gorm:"size:255;not null;default:''"`
	Icon           string         `gorm:"size:64;not null;default:''"`
	PermissionCode *string        `gorm:"size:128"`
	SortOrder      int            `gorm:"not null;default:0"`
	Hidden         bool           `gorm:"not null;default:false"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}

func (Menu) TableName() string { return "menus" }
