package model

import (
	"time"

	"gorm.io/plugin/soft_delete"
)

// 文章状态常量。
const (
	ArticleDraft     = "draft"
	ArticlePublished = "published"
	ArticleArchived  = "archived"
)

// Article 文章模型。
type Article struct {
	Base
	Title       string                `gorm:"size:255;not null" json:"title"`
	Slug        string                `gorm:"size:255;not null;uniqueIndex:idx_article_slug,priority:1" json:"slug"`
	Summary     string                `gorm:"size:500" json:"summary"`
	Content     string                `gorm:"type:longtext" json:"content"`
	CoverImage  string                `gorm:"size:255" json:"cover_image"`
	Status      string                `gorm:"size:16;not null;default:draft;index" json:"status"`
	ViewCount   int64                 `gorm:"not null;default:0" json:"view_count"`
	IsPinned    bool                  `gorm:"not null;default:false;index" json:"is_pinned"`
	IsFeatured  bool                  `gorm:"not null;default:false;index" json:"is_featured"`
	AuthorID    string                `gorm:"size:36;not null;index" json:"author_id"`
	Author      *User                 `gorm:"foreignKey:AuthorID" json:"author,omitempty"`
	CategoryID  string                `gorm:"size:36;index" json:"category_id"`
	Category    *Category             `gorm:"foreignKey:CategoryID" json:"category,omitempty"`
	Tags        []Tag                 `gorm:"many2many:article_tags;" json:"tags,omitempty"`
	PublishedAt *time.Time            `json:"published_at,omitempty"`
	DeletedAt   soft_delete.DeletedAt `gorm:"uniqueIndex:idx_article_slug,priority:2" json:"-"`
}
