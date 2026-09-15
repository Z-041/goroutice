package service

import (
	"errors"

	"goroutice/internal/dto"
	"goroutice/internal/model"
	"goroutice/internal/pkg/apperror"
	"goroutice/internal/repository"

	"gorm.io/gorm"
)

// UserService 用户管理服务（管理员）。
type UserService struct {
	userRepo *repository.UserRepository
	roleRepo *repository.UserRoleRepository
}

// NewUserService 构造 UserService。
func NewUserService(userRepo *repository.UserRepository, roleRepo *repository.UserRoleRepository) *UserService {
	return &UserService{userRepo: userRepo, roleRepo: roleRepo}
}

// List 分页查询用户。
func (s *UserService) List(page, size int, keyword string) ([]dto.UserInfo, int64, error) {
	users, total, err := s.userRepo.List(page, size, keyword)
	if err != nil {
		return nil, 0, err
	}

	infos := make([]dto.UserInfo, 0, len(users))
	for i := range users {
		roles, err := s.roleRepo.GetRolesByUserID(users[i].ID)
		if err != nil {
			return nil, 0, err
		}
		infos = append(infos, *dto.ToUserInfoWithRoles(&users[i], roles))
	}
	return infos, total, nil
}

// Update 管理员更新用户状态/昵称（角色分配走 PolicyService.AssignRole）。
func (s *UserService) Update(id string, req dto.UpdateUserRequest) (*dto.UserInfo, error) {
	user, err := s.userRepo.GetByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.NotFound("user not found")
		}
		return nil, err
	}

	fields := map[string]any{}
	if req.Status != nil {
		if *req.Status != model.StatusActive && *req.Status != model.StatusDisabled {
			return nil, apperror.BadRequest("invalid status")
		}
		fields["status"] = *req.Status
		user.Status = *req.Status
	}
	if req.Nickname != "" {
		fields["nickname"] = req.Nickname
		user.Nickname = req.Nickname
	}

	if err := s.userRepo.UpdateFields(id, fields); err != nil {
		return nil, err
	}
	roles, err := s.roleRepo.GetRolesByUserID(id)
	if err != nil {
		return nil, err
	}
	return dto.ToUserInfoWithRoles(user, roles), nil
}

// Delete 软删除用户。
func (s *UserService) Delete(id string) error {
	if _, err := s.userRepo.GetByID(id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperror.NotFound("user not found")
		}
		return err
	}
	return s.userRepo.Delete(id)
}
