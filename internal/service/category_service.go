package service

import (
	"errors"

	"goroutice/internal/dto"
	"goroutice/internal/model"
	"goroutice/internal/pkg/apperror"
	"goroutice/internal/repository"

	"gorm.io/gorm"
)

// CategoryService 分类服务。
type CategoryService struct {
	repo *repository.CategoryRepository
}

// NewCategoryService 构造 CategoryService。
func NewCategoryService(repo *repository.CategoryRepository) *CategoryService {
	return &CategoryService{repo: repo}
}

// Create 创建分类。
func (s *CategoryService) Create(req dto.CategoryRequest) (*dto.CategoryInfo, error) {
	if _, err := s.repo.GetByName(req.Name); err == nil {
		return nil, apperror.Conflict("category name already exists")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	base, err := taxonomySlug(req.Name, req.Slug)
	if err != nil {
		return nil, err
	}
	slugStr, err := resolveSlug(base, taxonomySlugMaxLen, func(slug string) (bool, error) {
		_, err := s.repo.GetBySlug(slug)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}

	c := &model.Category{
		Name:        req.Name,
		Slug:        slugStr,
		Description: req.Description,
		Sort:        req.Sort,
	}
	if err := s.repo.Create(c); err != nil {
		// 并发创建同名分类时唯一索引兜底：翻译成 409 而不是 500。
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, apperror.Conflict("category name or slug already exists")
		}
		return nil, err
	}
	return dto.ToCategoryInfo(c), nil
}

// Update 更新分类。
func (s *CategoryService) Update(id string, req dto.CategoryRequest) (*dto.CategoryInfo, error) {
	c, err := s.repo.GetByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.NotFound("category not found")
		}
		return nil, err
	}

	if req.Name != "" && req.Name != c.Name {
		if _, err := s.repo.GetByName(req.Name); err == nil {
			return nil, apperror.Conflict("category name already exists")
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		c.Name = req.Name
	}

	if req.Slug != "" && req.Slug != c.Slug {
		base, err := taxonomySlug(req.Name, req.Slug)
		if err != nil {
			return nil, err
		}
		slugStr, err := resolveSlug(base, taxonomySlugMaxLen, func(slug string) (bool, error) {
			existing, err := s.repo.GetBySlug(slug)
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return false, nil
			}
			if err != nil {
				return false, err
			}
			return existing.ID != id, nil
		})
		if err != nil {
			return nil, err
		}
		c.Slug = slugStr
	}

	c.Description = req.Description
	c.Sort = req.Sort

	if err := s.repo.Update(c); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, apperror.Conflict("category name or slug already exists")
		}
		return nil, err
	}
	return dto.ToCategoryInfo(c), nil
}

// GetByID 按 ID 查询分类。
func (s *CategoryService) GetByID(id string) (*dto.CategoryInfo, error) {
	c, err := s.repo.GetByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.NotFound("category not found")
		}
		return nil, err
	}
	return dto.ToCategoryInfo(c), nil
}

// GetBySlug 按 slug 查询分类。
func (s *CategoryService) GetBySlug(slug string) (*dto.CategoryInfo, error) {
	c, err := s.repo.GetBySlug(slug)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.NotFound("category not found")
		}
		return nil, err
	}
	return dto.ToCategoryInfo(c), nil
}

// List 分页查询分类。
func (s *CategoryService) List(page, size int, keyword string) ([]dto.CategoryInfo, int64, error) {
	categories, total, err := s.repo.List(page, size, keyword)
	if err != nil {
		return nil, 0, err
	}

	infos := make([]dto.CategoryInfo, 0, len(categories))
	for i := range categories {
		infos = append(infos, *dto.ToCategoryInfo(&categories[i]))
	}
	return infos, total, nil
}

// Delete 删除分类，若存在引用该分类的文章则拒绝删除。
func (s *CategoryService) Delete(id string) error {
	deleted, err := s.repo.DeleteIfUnused(id)
	if err != nil {
		return err
	}
	if deleted {
		return nil
	}

	// 未删除：区分「分类不存在」与「仍被文章引用」。
	if _, err := s.repo.GetByID(id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperror.NotFound("category not found")
		}
		return err
	}
	return apperror.Conflict("category is in use by articles")
}
