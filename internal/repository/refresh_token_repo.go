package repository

import (
	"time"

	"goroutice/internal/model"

	"gorm.io/gorm"
)

// RefreshTokenRepository 管理刷新令牌记录（仅存哈希）。
type RefreshTokenRepository struct {
	db *gorm.DB
}

// NewRefreshTokenRepository 构造 RefreshTokenRepository。
func NewRefreshTokenRepository(db *gorm.DB) *RefreshTokenRepository {
	return &RefreshTokenRepository{db: db}
}

// Create 保存一条刷新令牌记录。
func (r *RefreshTokenRepository) Create(rt *model.RefreshToken) error {
	return r.db.Create(rt).Error
}

// GetByHash 按令牌哈希查询记录。
func (r *RefreshTokenRepository) GetByHash(hash string) (*model.RefreshToken, error) {
	var rt model.RefreshToken
	err := r.db.Where("token_hash = ?", hash).First(&rt).Error
	return &rt, err
}

// DeleteByHash 删除指定令牌记录（登出时吊销）。
func (r *RefreshTokenRepository) DeleteByHash(hash string) error {
	return r.db.Where("token_hash = ?", hash).Delete(&model.RefreshToken{}).Error
}

// ConsumeByHash 以单条带条件的删除原子消费令牌，返回值表示本次调用是否抢占成功。
// 并发携带同一 refresh token 时只有一个调用能返回 true，避免令牌被重复兑换。
func (r *RefreshTokenRepository) ConsumeByHash(hash string, now time.Time) (bool, error) {
	res := r.db.Where("token_hash = ? AND expires_at > ?", hash, now).Delete(&model.RefreshToken{})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// DeleteByUserID 删除某用户的全部令牌（全端下线/注销时）。
func (r *RefreshTokenRepository) DeleteByUserID(userID string) error {
	return r.db.Where("user_id = ?", userID).Delete(&model.RefreshToken{}).Error
}

// DeleteExpired 清理已过期的令牌记录。
func (r *RefreshTokenRepository) DeleteExpired(now time.Time) error {
	return r.db.Where("expires_at < ?", now).Delete(&model.RefreshToken{}).Error
}
