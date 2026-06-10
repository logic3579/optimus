package models

import "time"

type CredentialCloudKey struct {
	ID                 uint64  `gorm:"primaryKey"`
	Name               string  `gorm:"size:128;not null;uniqueIndex"`
	Description        string  `gorm:"type:text;not null;default:''"`
	Provider           string  `gorm:"size:16;not null"`
	Region             string  `gorm:"size:32;not null;default:''"`
	AccessKeyIDEnc     []byte  `gorm:"column:access_key_id_enc;not null"`
	SecretAccessKeyEnc []byte  `gorm:"column:secret_access_key_enc;not null"`
	CreatedByUserID    *uint64 `gorm:"column:created_by_user_id"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (CredentialCloudKey) TableName() string { return "credentials_cloud_keys" }
