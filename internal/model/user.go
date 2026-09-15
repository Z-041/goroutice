package model

import (
	"gorm.io/plugin/soft_delete"
)

// 用户角色常量。
const (
	RoleAdmin  = "admin"
	RoleAuthor = "author"
	RoleUser   = "user"
)

// 用户状态常量。
const (
	StatusDisabled   = 0
	StatusActive     = 1
	StatusUnverified = 2
)

// User 用户模型。
type User struct {
	Base
	Username     string                `gorm:"size:64;not null;uniqueIndex:idx_user_username,priority:1" json:"username"`
	Email        string                `gorm:"size:128;not null;uniqueIndex:idx_user_email,priority:1" json:"email"`
	PasswordHash string                `gorm:"size:255;not null" json:"-"`
	Nickname     string                `gorm:"size:64" json:"nickname"`
	Avatar       string                `gorm:"size:255" json:"avatar"`
	Bio          string                `gorm:"size:255" json:"bio"`
	Role         string                `gorm:"size:16;not null;default:user;index" json:"role"`
	Status       int                   `gorm:"not null;default:1" json:"status"`
	TokenVersion int                   `gorm:"not null;default:0" json:"-"`
	DeletedAt    soft_delete.DeletedAt `gorm:"uniqueIndex:idx_user_username,priority:2;uniqueIndex:idx_user_email,priority:2" json:"-"`
}
