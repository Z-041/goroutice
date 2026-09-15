package dto

import (
	"time"

	"goroutice/internal/model"
)

// UserInfo 用户信息响应。
type UserInfo struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	// Email 只在账号自助与管理端接口下发；公开文章里的作者信息会清空该字段。
	Email     string    `json:"email,omitempty"`
	Nickname  string    `json:"nickname"`
	Avatar    string    `json:"avatar"`
	Bio       string    `json:"bio"`
	Role      string    `json:"role"`
	Roles     []string  `json:"roles"`
	Status    int       `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// ToUserInfo 将用户模型转换为响应结构（Roles 需通过 ToUserInfoWithRoles 填充）。
func ToUserInfo(u *model.User) *UserInfo {
	if u == nil {
		return nil
	}
	return &UserInfo{
		ID:        u.ID,
		Username:  u.Username,
		Email:     u.Email,
		Nickname:  u.Nickname,
		Avatar:    u.Avatar,
		Bio:       u.Bio,
		Role:      u.Role,
		Status:    u.Status,
		CreatedAt: u.CreatedAt,
	}
}

// ToUserInfoWithRoles 将用户模型与完整角色列表转换为响应结构。
func ToUserInfoWithRoles(u *model.User, roles []string) *UserInfo {
	info := ToUserInfo(u)
	if info == nil {
		return nil
	}
	info.Roles = roles
	return info
}

// UpdateProfileRequest 更新个人资料请求。
// Avatar/Bio 用指针区分「未提交」与「显式置空」，避免只改昵称时把头像与简介清空。
type UpdateProfileRequest struct {
	Email    string  `json:"email" binding:"omitempty,email,max=128"`
	Nickname string  `json:"nickname" binding:"max=64"`
	Avatar   *string `json:"avatar" binding:"omitempty,max=255"`
	Bio      *string `json:"bio" binding:"omitempty,max=255"`
}

// UpdateUserRequest 管理员更新用户请求（角色分配走独立的 /users/:id/role 接口）。
type UpdateUserRequest struct {
	Status   *int   `json:"status"`
	Nickname string `json:"nickname" binding:"max=64"`
}
