package repository

import (
	"time"

	"goroutice/internal/model"

	"gorm.io/gorm"
)

// PasswordResetRepository 管理密码重置令牌。
type PasswordResetRepository struct {
	db *gorm.DB
}

// NewPasswordResetRepository 构造 PasswordResetRepository。
func NewPasswordResetRepository(db *gorm.DB) *PasswordResetRepository {
	return &PasswordResetRepository{db: db}
}

// Create 保存一条重置令牌。
func (r *PasswordResetRepository) Create(pr *model.PasswordReset) error {
	return r.db.Create(pr).Error
}

// GetByToken 按令牌哈希查询记录（入库的即为哈希，调用方需先对明文令牌做哈希）。
func (r *PasswordResetRepository) GetByToken(tokenHash string) (*model.PasswordReset, error) {
	var pr model.PasswordReset
	err := r.db.Where("token = ?", tokenHash).First(&pr).Error
	return &pr, err
}

// MarkUsed 将令牌标记为已使用；返回值表示是否由本次调用完成标记。
// 条件更新保证并发消费同一令牌时只有一个调用成功。
func (r *PasswordResetRepository) MarkUsed(id string, now time.Time) (bool, error) {
	res := r.db.Model(&model.PasswordReset{}).
		Where("id = ? AND used_at IS NULL", id).
		Update("used_at", now)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// DeleteByUserID 删除某用户的全部重置令牌。
func (r *PasswordResetRepository) DeleteByUserID(userID string) error {
	return r.db.Where("user_id = ?", userID).Delete(&model.PasswordReset{}).Error
}

// DeleteExpired 清理已过期的重置令牌。
func (r *PasswordResetRepository) DeleteExpired(now time.Time) error {
	return r.db.Where("expires_at < ?", now).Delete(&model.PasswordReset{}).Error
}
