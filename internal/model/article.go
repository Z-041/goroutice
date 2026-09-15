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
	Title      string `gorm:"size:255;not null" json:"title"`
	Slug       string `gorm:"size:255;not null;uniqueIndex:idx_article_slug,priority:1" json:"slug"`
	Summary    string `gorm:"size:500" json:"summary"`
	Content    string `gorm:"type:longtext" json:"content"`
	CoverImage string `gorm:"size:255" json:"cover_image"`
	Status     string `gorm:"size:16;not null;default:draft;index" json:"status"`
	ViewCount  int64  `gorm:"not null;default:0" json:"view_count"`
	// Version 是内容版本号，创建即为 1，每次内容更新 +1，用于乐观锁。
	// 客户端回传读取时拿到的值，服务端比对失败即拒绝写入，避免两人同时编辑时后提交的覆盖先提交的。
	// 只有 ArticleRequest 能覆盖的字段发生变更时才自增（见 ArticleRepository.Update / UpdateStatus）；
	// 0 保留给「未提供版本」，因此从 1 起算，老数据经迁移也会被补成 1。
	Version     int                   `gorm:"not null;default:1" json:"version"`
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
