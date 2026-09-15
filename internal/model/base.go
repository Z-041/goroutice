package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Base 是所有模型的公共字段，主键使用 UUID 字符串。
type Base struct {
	ID        string    `gorm:"primarykey;size:36" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BeforeCreate 在创建前为主键生成 UUID（若调用方未显式指定）。
func (b *Base) BeforeCreate(tx *gorm.DB) error {
	if b.ID == "" {
		b.ID = uuid.NewString()
	}
	return nil
}
