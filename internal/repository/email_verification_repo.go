package repository

import (
	"time"

	"goroutice/internal/model"

	"gorm.io/gorm"
)

// EmailVerificationRepository 管理邮箱验证令牌。
type EmailVerificationRepository struct {
	db *gorm.DB
}

// NewEmailVerificationRepository 构造 EmailVerificationRepository。
func NewEmailVerificationRepository(db *gorm.DB) *EmailVerificationRepository {
	return &EmailVerificationRepository{db: db}
}

// Create 保存一条验证令牌。
func (r *EmailVerificationRepository) Create(ev *model.EmailVerification) error {
	return r.db.Create(ev).Error
}

// GetByToken 按令牌哈希查询记录（入库的即为哈希，调用方需先对明文令牌做哈希）。
func (r *EmailVerificationRepository) GetByToken(tokenHash string) (*model.EmailVerification, error) {
	var ev model.EmailVerification
	err := r.db.Where("token = ?", tokenHash).First(&ev).Error
	return &ev, err
}

// MarkUsed 将令牌标记为已使用；返回值表示是否由本次调用完成标记。
// 条件更新保证并发消费同一令牌时只有一个调用成功。
func (r *EmailVerificationRepository) MarkUsed(id string, now time.Time) (bool, error) {
	res := r.db.Model(&model.EmailVerification{}).
		Where("id = ? AND used_at IS NULL", id).
		Update("used_at", now)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// DeleteByUserID 删除某用户的全部验证令牌。
func (r *EmailVerificationRepository) DeleteByUserID(userID string) error {
	return r.db.Where("user_id = ?", userID).Delete(&model.EmailVerification{}).Error
}

// DeleteExpired 清理已过期的验证令牌。
func (r *EmailVerificationRepository) DeleteExpired(now time.Time) error {
	return r.db.Where("expires_at < ?", now).Delete(&model.EmailVerification{}).Error
}
