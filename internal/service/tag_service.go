package service

import (
	"errors"

	"goroutice/internal/dto"
	"goroutice/internal/model"
	"goroutice/internal/pkg/apperror"
	"goroutice/internal/repository"

	"gorm.io/gorm"
)

// TagService 标签服务。
type TagService struct {
	repo *repository.TagRepository
}

// NewTagService 构造 TagService。
func NewTagService(repo *repository.TagRepository) *TagService {
	return &TagService{repo: repo}
}

// Create 创建标签。
func (s *TagService) Create(req dto.TagRequest) (*dto.TagInfo, error) {
	if _, err := s.repo.GetByName(req.Name); err == nil {
		return nil, apperror.Conflict("tag name already exists")
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

	t := &model.Tag{Name: req.Name, Slug: slugStr}
	if err := s.repo.Create(t); err != nil {
		// 并发创建同名标签时唯一索引兜底：翻译成 409 而不是 500。
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, apperror.Conflict("tag name or slug already exists")
		}
		return nil, err
	}
	return dto.ToTagInfo(t), nil
}

// Update 更新标签。
func (s *TagService) Update(id string, req dto.TagRequest) (*dto.TagInfo, error) {
	t, err := s.repo.GetByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.NotFound("tag not found")
		}
		return nil, err
	}

	if req.Name != "" && req.Name != t.Name {
		if _, err := s.repo.GetByName(req.Name); err == nil {
			return nil, apperror.Conflict("tag name already exists")
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		t.Name = req.Name
	}

	if req.Slug != "" && req.Slug != t.Slug {
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
		t.Slug = slugStr
	}

	if err := s.repo.Update(t); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, apperror.Conflict("tag name or slug already exists")
		}
		return nil, err
	}
	return dto.ToTagInfo(t), nil
}

// GetByID 按 ID 查询标签。
func (s *TagService) GetByID(id string) (*dto.TagInfo, error) {
	t, err := s.repo.GetByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.NotFound("tag not found")
		}
		return nil, err
	}
	return dto.ToTagInfo(t), nil
}

// List 分页查询标签。
func (s *TagService) List(page, size int, keyword string) ([]dto.TagInfo, int64, error) {
	tags, total, err := s.repo.List(page, size, keyword)
	if err != nil {
		return nil, 0, err
	}

	infos := make([]dto.TagInfo, 0, len(tags))
	for i := range tags {
		infos = append(infos, *dto.ToTagInfo(&tags[i]))
	}
	return infos, total, nil
}

// Delete 删除标签。
func (s *TagService) Delete(id string) error {
	if _, err := s.repo.GetByID(id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperror.NotFound("tag not found")
		}
		return err
	}
	return s.repo.Delete(id)
}
