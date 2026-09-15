package dto

import (
	"time"

	"goroutice/internal/model"
)

// CategoryInfo 分类响应。
type CategoryInfo struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description"`
	Sort        int       `json:"sort"`
	CreatedAt   time.Time `json:"created_at"`
}

// ToCategoryInfo 将分类模型转换为响应结构。
func ToCategoryInfo(c *model.Category) *CategoryInfo {
	if c == nil {
		return nil
	}
	return &CategoryInfo{
		ID:          c.ID,
		Name:        c.Name,
		Slug:        c.Slug,
		Description: c.Description,
		Sort:        c.Sort,
		CreatedAt:   c.CreatedAt,
	}
}

// CategoryRequest 创建/更新分类请求。
type CategoryRequest struct {
	Name        string `json:"name" binding:"required,max=64"`
	Slug        string `json:"slug" binding:"max=64"`
	Description string `json:"description" binding:"max=255"`
	Sort        int    `json:"sort"`
}
