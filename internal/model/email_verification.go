package model

import "time"

// EmailVerification 邮箱验证令牌。
// Token 保存的是令牌的 SHA-256 哈希（列名保留 token 以免迁移产生冗余列），明文只出现在邮件里。
type EmailVerification struct {
	Base
	UserID    string     `gorm:"size:36;not null;index" json:"user_id"`
	Token     string     `gorm:"size:64;not null;uniqueIndex" json:"-"`
	ExpiresAt time.Time  `gorm:"not null" json:"-"`
	UsedAt    *time.Time `json:"-"`
}
