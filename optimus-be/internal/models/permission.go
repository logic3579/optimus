package models

import "time"

type Permission struct {
	ID          uint64 `gorm:"primaryKey"`
	Code        string `gorm:"size:128;not null;uniqueIndex"`
	Name        string `gorm:"size:128;not null"`
	Category    string `gorm:"size:64;not null;default:''"`
	Description string `gorm:"size:512;not null;default:''"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (Permission) TableName() string { return "permissions" }
