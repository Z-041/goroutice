package repository

import (
	"goroutice/internal/model"

	"gorm.io/gorm"
)

// FileRepository 文件数据访问。
type FileRepository struct {
	db *gorm.DB
}

// NewFileRepository 构造 FileRepository。
func NewFileRepository(db *gorm.DB) *FileRepository {
	return &FileRepository{db: db}
}

// Create 保存文件元数据。
func (r *FileRepository) Create(f *model.File) error {
	return r.db.Create(f).Error
}

// GetByID 按 ID 查询文件。
func (r *FileRepository) GetByID(id string) (*model.File, error) {
	var f model.File
	err := r.db.Where("id = ?", id).First(&f).Error
	return &f, err
}

// Delete 删除文件元数据。
func (r *FileRepository) Delete(id string) error {
	return r.db.Where("id = ?", id).Delete(&model.File{}).Error
}

// List 分页查询文件，可按上传者过滤。
func (r *FileRepository) List(page, size int, uploaderID string) ([]model.File, int64, error) {
	q := r.db.Model(&model.File{})
	if uploaderID != "" {
		q = q.Where("uploader_id = ?", uploaderID)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var files []model.File
	err := q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&files).Error
	return files, total, err
}
