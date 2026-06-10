package models

import "time"

type CredentialSSHKey struct {
	ID              uint64  `gorm:"primaryKey"`
	Name            string  `gorm:"size:128;not null;uniqueIndex"`
	Description     string  `gorm:"type:text;not null;default:''"`
	Username        string  `gorm:"size:64;not null"`
	PrivateKeyEnc   []byte  `gorm:"column:private_key_enc;not null"`
	PassphraseEnc   []byte  `gorm:"column:passphrase_enc"`
	CreatedByUserID *uint64 `gorm:"column:created_by_user_id"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (CredentialSSHKey) TableName() string { return "credentials_ssh_keys" }
