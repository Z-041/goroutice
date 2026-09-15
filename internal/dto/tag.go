package dto

import (
	"time"

	"goroutice/internal/model"
)

// TagInfo 标签响应。
type TagInfo struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
}

// ToTagInfo 将标签模型转换为响应结构。
func ToTagInfo(t *model.Tag) *TagInfo {
	if t == nil {
		return nil
	}
	return &TagInfo{
		ID:        t.ID,
		Name:      t.Name,
		Slug:      t.Slug,
		CreatedAt: t.CreatedAt,
	}
}

// TagRequest 创建/更新标签请求。
type TagRequest struct {
	Name string `json:"name" binding:"required,max=64"`
	Slug string `json:"slug" binding:"max=64"`
}
