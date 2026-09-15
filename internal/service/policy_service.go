package service

import (
	"errors"
	"fmt"

	"goroutice/internal/dto"
	"goroutice/internal/model"
	"goroutice/internal/pkg/apperror"
	"goroutice/internal/repository"

	"github.com/casbin/casbin/v3"
	"gorm.io/gorm"
)

// PolicyService 管理 Casbin 权限策略（p）与角色继承（g），以及用户角色分配，并记录权限审计。
type PolicyService struct {
	enforcer *casbin.Enforcer
	userRepo *repository.UserRepository
	roleRepo *repository.UserRoleRepository
	auditor  *AuditService
}

// NewPolicyService 构造 PolicyService。
func NewPolicyService(enforcer *casbin.Enforcer, userRepo *repository.UserRepository, roleRepo *repository.UserRoleRepository, auditor *AuditService) *PolicyService {
	return &PolicyService{enforcer: enforcer, userRepo: userRepo, roleRepo: roleRepo, auditor: auditor}
}

// ListPolicies 返回权限策略与角色继承列表。
func (s *PolicyService) ListPolicies() ([]dto.PolicyItem, []dto.RoleInheritance, error) {
	ps, err := s.enforcer.GetPolicy()
	if err != nil {
		return nil, nil, err
	}
	gs, err := s.enforcer.GetGroupingPolicy()
	if err != nil {
		return nil, nil, err
	}

	policies := make([]dto.PolicyItem, 0, len(ps))
	for _, rule := range ps {
		item := dto.PolicyItem{}
		if len(rule) > 0 {
			item.Sub = rule[0]
		}
		if len(rule) > 1 {
			item.Obj = rule[1]
		}
		if len(rule) > 2 {
			item.Act = rule[2]
		}
		policies = append(policies, item)
	}

	roles := make([]dto.RoleInheritance, 0, len(gs))
	for _, rule := range gs {
		item := dto.RoleInheritance{}
		if len(rule) > 0 {
			item.Child = rule[0]
		}
		if len(rule) > 1 {
			item.Parent = rule[1]
		}
		roles = append(roles, item)
	}

	return policies, roles, nil
}

// AddPolicy 新增权限策略（幂等：已存在则报冲突）。
func (s *PolicyService) AddPolicy(operator Operator, sub, obj, act string) error {
	added, err := s.enforcer.AddPolicy(sub, obj, act)
	if err != nil {
		return err
	}
	if !added {
		return apperror.Conflict("policy already exists")
	}
	s.auditor.Record(operator, model.AuditPolicyAdd, fmt.Sprintf("sub=%s obj=%s act=%s", sub, obj, act))
	return nil
}

// RemovePolicy 删除权限策略。
func (s *PolicyService) RemovePolicy(operator Operator, sub, obj, act string) error {
	removed, err := s.enforcer.RemovePolicy(sub, obj, act)
	if err != nil {
		return err
	}
	if !removed {
		return apperror.NotFound("policy not found")
	}
	s.auditor.Record(operator, model.AuditPolicyRemove, fmt.Sprintf("sub=%s obj=%s act=%s", sub, obj, act))
	return nil
}

// AddRoleInheritance 新增角色继承（child 继承 parent）。
func (s *PolicyService) AddRoleInheritance(operator Operator, child, parent string) error {
	added, err := s.enforcer.AddGroupingPolicy(child, parent)
	if err != nil {
		return err
	}
	if !added {
		return apperror.Conflict("role inheritance already exists")
	}
	s.auditor.Record(operator, model.AuditRoleAdd, fmt.Sprintf("child=%s parent=%s", child, parent))
	return nil
}

// RemoveRoleInheritance 删除角色继承。
func (s *PolicyService) RemoveRoleInheritance(operator Operator, child, parent string) error {
	removed, err := s.enforcer.RemoveGroupingPolicy(child, parent)
	if err != nil {
		return err
	}
	if !removed {
		return apperror.NotFound("role inheritance not found")
	}
	s.auditor.Record(operator, model.AuditRoleRemove, fmt.Sprintf("child=%s parent=%s", child, parent))
	return nil
}

// AssignRole 分配用户角色集合。
func (s *PolicyService) AssignRole(operator Operator, userID string, roles []string) error {
	if len(roles) == 0 {
		return apperror.BadRequest("roles must not be empty")
	}

	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperror.NotFound("user not found")
		}
		return err
	}

	// 主角色取首个，用于用户列表展示与向后兼容；与 user_roles 在同一事务内更新。
	// 同时递增 token 版本：鉴权用的是 JWT 里的角色快照，不失效旧令牌则降权在令牌过期前不生效。
	fields := tokenVersionBump()
	fields["role"] = roles[0]
	if err := s.userRepo.UpdateWithRoles(userID, fields, roles); err != nil {
		return err
	}
	user.Role = roles[0]
	s.auditor.Record(operator, model.AuditAssignRole, fmt.Sprintf("user=%s roles=%v", user.Username, roles))
	return nil
}

// ListAudits 分页查询权限审计记录。
func (s *PolicyService) ListAudits(page, size int, keyword string) ([]dto.AuditInfo, int64, error) {
	return s.auditor.List(page, size, keyword)
}
