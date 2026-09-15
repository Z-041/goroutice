package repository

import (
	"goroutice/internal/model"

	"gorm.io/gorm"
)

// ArticleRevisionRepository 文章修订数据访问。
type ArticleRevisionRepository struct {
	db *gorm.DB
}

// NewArticleRevisionRepository 构造 ArticleRevisionRepository。
func NewArticleRevisionRepository(db *gorm.DB) *ArticleRevisionRepository {
	return &ArticleRevisionRepository{db: db}
}

// List 分页查询某篇文章的修订，按版本号倒序（最新的在前）。
//
// 不查正文：修订列表只用于让人挑出要回溯的版本，正文是 longtext，
// 一个 20 条的列表就能把几十万字符搬进内存。
func (r *ArticleRevisionRepository) List(articleID string, page, size int) ([]model.ArticleRevision, int64, error) {
	q := r.db.Model(&model.ArticleRevision{}).
		Select("id", "article_id", "editor_id", "version", "title", "slug", "status", "created_at").
		Where("article_id = ?", articleID)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var revisions []model.ArticleRevision
	err := q.Order("version DESC").Offset((page - 1) * size).Limit(size).Find(&revisions).Error
	return revisions, total, err
}

// GetByID 按 ID 查询单条修订（含正文）。
func (r *ArticleRevisionRepository) GetByID(id string) (*model.ArticleRevision, error) {
	var rev model.ArticleRevision
	err := r.db.Where("id = ?", id).First(&rev).Error
	return &rev, err
}
