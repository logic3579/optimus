package models

import "time"

type RolePermission struct {
	RoleID       uint64 `gorm:"primaryKey"`
	PermissionID uint64 `gorm:"primaryKey;column:permission_id"`
	CreatedAt    time.Time
}

func (RolePermission) TableName() string { return "role_permissions" }
