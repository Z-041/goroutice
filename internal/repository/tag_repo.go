package repository

import (
	"goroutice/internal/model"

	"gorm.io/gorm"
)

// TagRepository 标签数据访问。
type TagRepository struct {
	db *gorm.DB
}

// NewTagRepository 构造 TagRepository。
func NewTagRepository(db *gorm.DB) *TagRepository {
	return &TagRepository{db: db}
}

// Create 创建标签。
func (r *TagRepository) Create(t *model.Tag) error {
	return r.db.Create(t).Error
}

// GetByID 按 ID 查询标签。
func (r *TagRepository) GetByID(id string) (*model.Tag, error) {
	var t model.Tag
	err := r.db.Where("id = ?", id).First(&t).Error
	return &t, err
}

// GetByName 按名称查询标签。
func (r *TagRepository) GetByName(name string) (*model.Tag, error) {
	var t model.Tag
	err := r.db.Where("name = ?", name).First(&t).Error
	return &t, err
}

// GetBySlug 按 slug 查询标签。
func (r *TagRepository) GetBySlug(slug string) (*model.Tag, error) {
	var t model.Tag
	err := r.db.Where("slug = ?", slug).First(&t).Error
	return &t, err
}

// CountByIDs 统计指定 ID 中实际存在的标签数量。
func (r *TagRepository) CountByIDs(ids []string) (int64, error) {
	var n int64
	err := r.db.Model(&model.Tag{}).Where("id IN ?", ids).Count(&n).Error
	return n, err
}

// Update 更新标签。
func (r *TagRepository) Update(t *model.Tag) error {
	return r.db.Save(t).Error
}

// Delete 删除标签，并清理文章与标签的关联记录。
func (r *TagRepository) Delete(id string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM article_tags WHERE tag_id = ?", id).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&model.Tag{}).Error
	})
}

// List 分页查询标签，可按名称模糊搜索。
func (r *TagRepository) List(page, size int, keyword string) ([]model.Tag, int64, error) {
	q := r.db.Model(&model.Tag{})
	if keyword != "" {
		q = q.Where("name LIKE ?", "%"+keyword+"%")
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var tags []model.Tag
	err := q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&tags).Error
	return tags, total, err
}
