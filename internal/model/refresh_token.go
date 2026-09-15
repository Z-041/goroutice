package model

import "time"

// RefreshToken 刷新令牌记录，仅存哈希，用于轮换与吊销。
type RefreshToken struct {
	Base
	UserID    string    `gorm:"size:36;not null;index" json:"user_id"`
	TokenHash string    `gorm:"size:64;not null;uniqueIndex" json:"-"`
	ExpiresAt time.Time `gorm:"not null" json:"-"`
}
