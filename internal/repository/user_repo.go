package repository

import (
	"goroutice/internal/model"

	"gorm.io/gorm"
)

// UserRepository 用户数据访问。
type UserRepository struct {
	db *gorm.DB
}

// NewUserRepository 构造 UserRepository。
func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

// CreateWithRoles 在事务中创建用户并写入初始角色，避免出现「有用户无角色」的中间态。
func (r *UserRepository) CreateWithRoles(u *model.User, roles []string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(u).Error; err != nil {
			return err
		}
		return replaceRoles(tx, u.ID, roles)
	})
}

// GetByID 按 ID 查询用户。
func (r *UserRepository) GetByID(id string) (*model.User, error) {
	var u model.User
	err := r.db.Where("id = ?", id).First(&u).Error
	return &u, err
}

// GetByUsername 按用户名查询用户。
func (r *UserRepository) GetByUsername(username string) (*model.User, error) {
	var u model.User
	err := r.db.Where("username = ?", username).First(&u).Error
	return &u, err
}

// GetByEmail 按邮箱查询用户。
func (r *UserRepository) GetByEmail(email string) (*model.User, error) {
	var u model.User
	err := r.db.Where("email = ?", email).First(&u).Error
	return &u, err
}

// UpdateFields 按字段更新用户。
// 不使用 Save 全字段回写：那会把内存快照里的 password_hash、token_version 等一并写回，
// 并发场景下会覆盖其它请求刚提交的修改。
func (r *UserRepository) UpdateFields(id string, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	return r.db.Model(&model.User{}).Where("id = ?", id).Updates(fields).Error
}

// UpdateWithRoles 在事务中更新用户字段并替换角色集合，保持两处角色数据一致。
func (r *UserRepository) UpdateWithRoles(id string, fields map[string]any, roles []string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := replaceRoles(tx, id, roles); err != nil {
			return err
		}
		if len(fields) == 0 {
			return nil
		}
		return tx.Model(&model.User{}).Where("id = ?", id).Updates(fields).Error
	})
}

// UpdatePasswordAndRevokeSessions 在事务内更新密码、递增 token 版本并吊销全部 refresh token。
// 三步分开写会留下中间态：要么「密码已改但旧会话仍可用」，要么「已返回错误却已改密、无法重试」。
func (r *UserRepository) UpdatePasswordAndRevokeSessions(id, passwordHash string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		fields := map[string]any{
			"password_hash": passwordHash,
			"token_version": gorm.Expr("token_version + 1"),
		}
		if err := tx.Model(&model.User{}).Where("id = ?", id).Updates(fields).Error; err != nil {
			return err
		}
		return tx.Where("user_id = ?", id).Delete(&model.RefreshToken{}).Error
	})
}

// Delete 软删除用户。
func (r *UserRepository) Delete(id string) error {
	return r.db.Where("id = ?", id).Delete(&model.User{}).Error
}

// List 分页查询用户，可按用户名/昵称/邮箱模糊搜索。
func (r *UserRepository) List(page, size int, keyword string) ([]model.User, int64, error) {
	q := r.db.Model(&model.User{})
	if keyword != "" {
		kw := "%" + keyword + "%"
		q = q.Where("username LIKE ? OR nickname LIKE ? OR email LIKE ?", kw, kw, kw)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var users []model.User
	err := q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&users).Error
	return users, total, err
}
