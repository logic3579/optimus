package models

import "time"

type UserRole struct {
	UserID    uint64    `gorm:"primaryKey"`
	RoleID    uint64    `gorm:"primaryKey"`
	CreatedAt time.Time
}

func (UserRole) TableName() string { return "user_roles" }
