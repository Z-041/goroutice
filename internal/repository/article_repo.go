package repository

import (
	"time"

	"goroutice/internal/model"

	"gorm.io/gorm"
)

// ArticleFilter 文章列表过滤条件。
type ArticleFilter struct {
	Keyword    string
	CategoryID string
	TagID      string
	AuthorID   string
	Status     string
}

// ArticleRepository 文章数据访问。
type ArticleRepository struct {
	db *gorm.DB
}

// NewArticleRepository 构造 ArticleRepository。
func NewArticleRepository(db *gorm.DB) *ArticleRepository {
	return &ArticleRepository{db: db}
}

// Create 在事务中创建文章并绑定标签。
func (r *ArticleRepository) Create(a *model.Article, tagIDs []string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(a).Error; err != nil {
			return err
		}
		if len(tagIDs) > 0 {
			if err := replaceTags(tx, a, tagIDs); err != nil {
				return err
			}
		}
		return nil
	})
}

// Update 在事务中更新文章可变字段并重建标签关联。
// 只写入白名单列，避免全字段回写覆盖并发自增的 view_count 与不可变的 created_at。
func (r *ArticleRepository) Update(a *model.Article, tagIDs []string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{
			"title":        a.Title,
			"slug":         a.Slug,
			"summary":      a.Summary,
			"content":      a.Content,
			"cover_image":  a.CoverImage,
			"status":       a.Status,
			"category_id":  a.CategoryID,
			"published_at": a.PublishedAt,
			"updated_at":   time.Now(),
		}
		if err := tx.Model(&model.Article{}).Where("id = ?", a.ID).Updates(updates).Error; err != nil {
			return err
		}
		return replaceTags(tx, a, tagIDs)
	})
}

// Delete 软删除文章，并清理标签关联记录。
// 关联表没有软删除列，不清理会在表里长期堆积指向已删除文章的孤儿行。
func (r *ArticleRepository) Delete(id string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM article_tags WHERE article_id = ?", id).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&model.Article{}).Error
	})
}

// GetByID 按 ID 查询文章并预加载关联。
func (r *ArticleRepository) GetByID(id string) (*model.Article, error) {
	var a model.Article
	err := r.db.Preload("Author").Preload("Category").Preload("Tags").Where("id = ?", id).First(&a).Error
	return &a, err
}

// GetBySlug 按 slug 查询文章并预加载关联。
func (r *ArticleRepository) GetBySlug(slug string) (*model.Article, error) {
	var a model.Article
	err := r.db.Preload("Author").Preload("Category").Preload("Tags").
		Where("slug = ?", slug).First(&a).Error
	return &a, err
}

// List 分页查询文章。
func (r *ArticleRepository) List(page, size int, f ArticleFilter) ([]model.Article, int64, error) {
	q := r.db.Model(&model.Article{}).Preload("Author").Preload("Category").Preload("Tags")

	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.CategoryID != "" {
		q = q.Where("category_id = ?", f.CategoryID)
	}
	if f.AuthorID != "" {
		q = q.Where("author_id = ?", f.AuthorID)
	}
	if f.Keyword != "" {
		kw := "%" + f.Keyword + "%"
		q = q.Where("title LIKE ? OR summary LIKE ? OR content LIKE ?", kw, kw, kw)
	}
	if f.TagID != "" {
		sub := r.db.Table("article_tags").Select("article_id").Where("tag_id = ?", f.TagID)
		q = q.Where("id IN (?)", sub)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var articles []model.Article
	err := q.Order("is_pinned DESC, is_featured DESC, created_at DESC").Offset((page - 1) * size).Limit(size).Find(&articles).Error
	return articles, total, err
}

// IncrementView 文章浏览数自增。
func (r *ArticleRepository) IncrementView(id string) error {
	return r.db.Model(&model.Article{}).Where("id = ?", id).
		UpdateColumn("view_count", gorm.Expr("view_count + ?", 1)).Error
}

// UpdateStatus 更新文章状态；发布时间只在首次发布时写入，重复发布不覆盖原值。
func (r *ArticleRepository) UpdateStatus(id string, status string) error {
	updates := map[string]interface{}{"status": status}
	if status == model.ArticlePublished {
		updates["published_at"] = gorm.Expr("COALESCE(published_at, ?)", time.Now())
	}
	return r.db.Model(&model.Article{}).Where("id = ?", id).Updates(updates).Error
}

// UpdateFeature 按需更新文章置顶/推荐标记，nil 表示保持原值，避免读-改-写丢失并发更新。
func (r *ArticleRepository) UpdateFeature(id string, isPinned, isFeatured *bool) error {
	updates := map[string]interface{}{}
	if isPinned != nil {
		updates["is_pinned"] = *isPinned
	}
	if isFeatured != nil {
		updates["is_featured"] = *isFeatured
	}
	if len(updates) == 0 {
		return nil
	}
	return r.db.Model(&model.Article{}).Where("id = ?", id).Updates(updates).Error
}

func replaceTags(tx *gorm.DB, a *model.Article, tagIDs []string) error {
	var tags []model.Tag
	if len(tagIDs) > 0 {
		if err := tx.Where("id IN ?", tagIDs).Find(&tags).Error; err != nil {
			return err
		}
	}
	return tx.Model(a).Association("Tags").Replace(tags)
}
