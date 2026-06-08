package models

import (
	"strconv"
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID           uint64 `gorm:"primaryKey"`
	Username     string `gorm:"size:64;not null"`
	Email        string `gorm:"size:128;not null"`
	PasswordHash string `gorm:"size:255;not null"`
	DisplayName  string `gorm:"size:128;not null;default:''"`
	AvatarURL    string `gorm:"size:512;not null;default:''"`
	Status       string `gorm:"size:16;not null;default:'enabled'"`
	LastLoginAt  *time.Time
	CreatedBy    *uint64
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    gorm.DeletedAt `gorm:"index"`
}

func (User) TableName() string { return "users" }

func (u User) IDString() string { return strconv.FormatUint(u.ID, 10) }
