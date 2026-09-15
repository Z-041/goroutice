package repository

import (
	"goroutice/internal/model"

	"gorm.io/gorm"
)

// UserRoleRepository 管理用户角色关联。
type UserRoleRepository struct {
	db *gorm.DB
}

// NewUserRoleRepository 构造 UserRoleRepository。
func NewUserRoleRepository(db *gorm.DB) *UserRoleRepository {
	return &UserRoleRepository{db: db}
}

// GetRolesByUserID 返回用户的全部角色（去重）。
func (r *UserRoleRepository) GetRolesByUserID(userID string) ([]string, error) {
	var roles []string
	err := r.db.Model(&model.UserRole{}).Where("user_id = ?", userID).Pluck("role", &roles).Error
	return roles, err
}

// ReplaceRoles 以事务方式替换用户的全部角色（去重）。
func (r *UserRoleRepository) ReplaceRoles(userID string, roles []string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return replaceRoles(tx, userID, roles)
	})
}

// replaceRoles 在给定事务中替换用户的角色集合（去重后一次性写入）。
func replaceRoles(tx *gorm.DB, userID string, roles []string) error {
	seen := make(map[string]struct{}, len(roles))
	rows := make([]model.UserRole, 0, len(roles))
	for _, role := range roles {
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		rows = append(rows, model.UserRole{UserID: userID, Role: role})
	}

	if err := tx.Where("user_id = ?", userID).Delete(&model.UserRole{}).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.Create(&rows).Error
}
