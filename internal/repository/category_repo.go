package repository

import (
	"goroutice/internal/model"

	"gorm.io/gorm"
)

// CategoryRepository 分类数据访问。
type CategoryRepository struct {
	db *gorm.DB
}

// NewCategoryRepository 构造 CategoryRepository。
func NewCategoryRepository(db *gorm.DB) *CategoryRepository {
	return &CategoryRepository{db: db}
}

// Create 创建分类。
func (r *CategoryRepository) Create(c *model.Category) error {
	return r.db.Create(c).Error
}

// GetByID 按 ID 查询分类。
func (r *CategoryRepository) GetByID(id string) (*model.Category, error) {
	var c model.Category
	err := r.db.Where("id = ?", id).First(&c).Error
	return &c, err
}

// GetByName 按名称查询分类。
func (r *CategoryRepository) GetByName(name string) (*model.Category, error) {
	var c model.Category
	err := r.db.Where("name = ?", name).First(&c).Error
	return &c, err
}

// GetBySlug 按 slug 查询分类。
func (r *CategoryRepository) GetBySlug(slug string) (*model.Category, error) {
	var c model.Category
	err := r.db.Where("slug = ?", slug).First(&c).Error
	return &c, err
}

// Update 更新分类。
func (r *CategoryRepository) Update(c *model.Category) error {
	return r.db.Save(c).Error
}

// DeleteIfUnused 在无文章引用时删除分类，返回值表示是否删除了记录。
// 用单条带条件的 DELETE 取代「先 count 再 delete」，避免两步之间被并发写入插入引用。
// 表名沿用 GORM 的默认命名（Category -> categories、Article -> articles），
// deleted_at = 0 为 gorm soft_delete 插件的未删除值。
func (r *CategoryRepository) DeleteIfUnused(id string) (bool, error) {
	res := r.db.Exec(
		`DELETE FROM categories WHERE id = ? AND NOT EXISTS (
			SELECT 1 FROM articles WHERE articles.category_id = categories.id AND articles.deleted_at = 0
		)`, id)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// List 分页查询分类，可按名称/描述模糊搜索。
func (r *CategoryRepository) List(page, size int, keyword string) ([]model.Category, int64, error) {
	q := r.db.Model(&model.Category{})
	if keyword != "" {
		kw := "%" + keyword + "%"
		q = q.Where("name LIKE ? OR description LIKE ?", kw, kw)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var categories []model.Category
	err := q.Order("sort ASC, id DESC").Offset((page - 1) * size).Limit(size).Find(&categories).Error
	return categories, total, err
}
