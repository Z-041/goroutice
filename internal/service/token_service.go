package service

import (
	"errors"
	"time"

	"goroutice/internal/dto"
	"goroutice/internal/model"
	"goroutice/internal/pkg/apperror"
	"goroutice/internal/pkg/jwt"
	"goroutice/internal/pkg/token"
	"goroutice/internal/repository"

	"gorm.io/gorm"
)

// TokenService 负责 refresh token 的签发、轮换与吊销，以及 access token 的签发。
type TokenService struct {
	refreshRepo   *repository.RefreshTokenRepository
	userRepo      *repository.UserRepository
	roleRepo      *repository.UserRoleRepository
	jwtMgr        *jwt.Manager
	refreshExpire time.Duration
}

// NewTokenService 构造 TokenService。
func NewTokenService(refreshRepo *repository.RefreshTokenRepository, userRepo *repository.UserRepository, roleRepo *repository.UserRoleRepository, jwtMgr *jwt.Manager, refreshExpireHours int) *TokenService {
	return &TokenService{
		refreshRepo:   refreshRepo,
		userRepo:      userRepo,
		roleRepo:      roleRepo,
		jwtMgr:        jwtMgr,
		refreshExpire: time.Duration(refreshExpireHours) * time.Hour,
	}
}

// IssueTokens 为用户签发一对令牌（access + refresh），refresh 仅存哈希。
func (s *TokenService) IssueTokens(userID, username string, roles []string, tokenVersion int) (*dto.TokenPair, error) {
	access, accessExp, err := s.jwtMgr.GenerateAccessToken(userID, username, roles, tokenVersion)
	if err != nil {
		return nil, err
	}

	refreshRaw, err := token.Generate()
	if err != nil {
		return nil, err
	}
	refreshExp := time.Now().Add(s.refreshExpire)

	rec := &model.RefreshToken{
		UserID:    userID,
		TokenHash: token.Hash(refreshRaw),
		ExpiresAt: refreshExp,
	}
	if err := s.refreshRepo.Create(rec); err != nil {
		return nil, err
	}

	return &dto.TokenPair{
		AccessToken:      access,
		AccessExpiresAt:  accessExp.Format(time.RFC3339),
		RefreshToken:     refreshRaw,
		RefreshExpiresAt: refreshExp.Format(time.RFC3339),
	}, nil
}

// Refresh 校验 refresh token 并轮换出一对新令牌，旧 refresh 同时吊销。
func (s *TokenService) Refresh(refreshToken string) (*dto.TokenPair, error) {
	rec, err := s.refreshRepo.GetByHash(token.Hash(refreshToken))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.Unauthorized("invalid refresh token")
		}
		return nil, err
	}
	if time.Now().After(rec.ExpiresAt) {
		return nil, apperror.Unauthorized("refresh token expired")
	}

	user, err := s.userRepo.GetByID(rec.UserID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.Unauthorized("user not found")
		}
		return nil, err
	}
	if user.Status != model.StatusActive {
		return nil, apperror.Forbidden("account is disabled")
	}

	roles, err := s.roleRepo.GetRolesByUserID(user.ID)
	if err != nil {
		return nil, err
	}

	// 轮换：原子消费旧 refresh，未抢占到（并发重放/已吊销/已过期）则不签发新令牌。
	consumed, err := s.refreshRepo.ConsumeByHash(rec.TokenHash, time.Now())
	if err != nil {
		return nil, err
	}
	if !consumed {
		return nil, apperror.Unauthorized("invalid refresh token")
	}
	return s.IssueTokens(user.ID, user.Username, roles, user.TokenVersion)
}

// Revoke 吊销单个 refresh token（登出）。
func (s *TokenService) Revoke(refreshToken string) error {
	return s.refreshRepo.DeleteByHash(token.Hash(refreshToken))
}

// RevokeAll 吊销某用户的全部 refresh token（全端下线/注销）。
func (s *TokenService) RevokeAll(userID string) error {
	return s.refreshRepo.DeleteByUserID(userID)
}
