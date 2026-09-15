package repository

import (
	"goroutice/internal/model"

	"gorm.io/gorm"
)

// PermissionAuditRepository 权限审计记录数据访问。
type PermissionAuditRepository struct {
	db *gorm.DB
}

// NewPermissionAuditRepository 构造 PermissionAuditRepository。
func NewPermissionAuditRepository(db *gorm.DB) *PermissionAuditRepository {
	return &PermissionAuditRepository{db: db}
}

// Create 写入一条审计记录。
func (r *PermissionAuditRepository) Create(entry *model.PermissionAudit) error {
	return r.db.Create(entry).Error
}

// List 分页查询审计记录，可按操作者/动作/详情模糊过滤。
func (r *PermissionAuditRepository) List(page, size int, keyword string) ([]model.PermissionAudit, int64, error) {
	q := r.db.Model(&model.PermissionAudit{})
	if keyword != "" {
		kw := "%" + keyword + "%"
		q = q.Where("operator_username LIKE ? OR action LIKE ? OR detail LIKE ?", kw, kw, kw)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var entries []model.PermissionAudit
	err := q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&entries).Error
	return entries, total, err
}
