package models

import (
	"time"

	"gorm.io/gorm"
)

type HTTPCredential struct {
	ID               uint64 `gorm:"primaryKey"`
	Name             string
	AuthType         string
	Username         *string
	SecretCiphertext []byte
	CreatedByUserID  *uint64
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        gorm.DeletedAt `gorm:"index"`
}

func (HTTPCredential) TableName() string { return "credentials_http_credentials" }
