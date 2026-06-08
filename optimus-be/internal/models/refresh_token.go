package models

import "time"

type RefreshToken struct {
	ID        uint64    `gorm:"primaryKey"`
	UserID    uint64    `gorm:"not null;index"`
	TokenHash string    `gorm:"size:64;not null;uniqueIndex"`
	ExpiresAt time.Time `gorm:"not null;index"`
	RevokedAt *time.Time
	UserAgent string `gorm:"size:512;not null;default:''"`
	IP        string `gorm:"size:64;not null;default:''"`
	CreatedAt time.Time
}

func (RefreshToken) TableName() string { return "refresh_tokens" }
