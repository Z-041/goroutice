package handler

import (
	"goroutice/internal/dto"
	"goroutice/internal/middleware"
	"goroutice/internal/pkg/response"
	"goroutice/internal/service"

	"github.com/gin-gonic/gin"
)

// AuthHandler 认证与账户处理器。
type AuthHandler struct {
	authService *service.AuthService
}

// NewAuthHandler 构造 AuthHandler。
func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

// Register 注册。
func (h *AuthHandler) Register(c *gin.Context) {
	var req dto.RegisterRequest
	if !bindJSON(c, &req) {
		return
	}

	user, err := h.authService.Register(req)
	if err != nil {
		handleError(c, err)
		return
	}
	response.Created(c, user)
}

// Login 登录。
func (h *AuthHandler) Login(c *gin.Context) {
	var req dto.LoginRequest
	if !bindJSON(c, &req) {
		return
	}

	resp, err := h.authService.Login(req)
	if err != nil {
		handleError(c, err)
		return
	}
	response.Success(c, resp)
}

// Refresh 刷新令牌。
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req dto.RefreshRequest
	if !bindJSON(c, &req) {
		return
	}

	pair, err := h.authService.Refresh(req)
	if err != nil {
		handleError(c, err)
		return
	}
	response.Success(c, pair)
}

// Logout 登出（吊销 refresh token）。
func (h *AuthHandler) Logout(c *gin.Context) {
	var req dto.LogoutRequest
	if !bindJSON(c, &req) {
		return
	}

	if err := h.authService.Logout(req); err != nil {
		handleError(c, err)
		return
	}
	response.Success(c, nil)
}

// LogoutAll 全端下线。
func (h *AuthHandler) LogoutAll(c *gin.Context) {
	if err := h.authService.LogoutAll(middleware.CurrentUserID(c)); err != nil {
		handleError(c, err)
		return
	}
	response.Success(c, nil)
}

// DeleteAccount 注销当前账号。
func (h *AuthHandler) DeleteAccount(c *gin.Context) {
	if err := h.authService.DeleteAccount(middleware.CurrentUserID(c)); err != nil {
		handleError(c, err)
		return
	}
	response.Success(c, nil)
}

// VerifyEmail 验证邮箱。
func (h *AuthHandler) VerifyEmail(c *gin.Context) {
	var req dto.VerifyEmailRequest
	if !bindJSON(c, &req) {
		return
	}

	if err := h.authService.VerifyEmail(req); err != nil {
		handleError(c, err)
		return
	}
	response.Success(c, nil)
}

// ResendVerification 重新发送验证邮件。
func (h *AuthHandler) ResendVerification(c *gin.Context) {
	var req dto.ResendVerificationRequest
	if !bindJSON(c, &req) {
		return
	}

	if err := h.authService.ResendVerification(req); err != nil {
		handleError(c, err)
		return
	}
	response.Success(c, nil)
}

// ForgotPassword 忘记密码。
func (h *AuthHandler) ForgotPassword(c *gin.Context) {
	var req dto.ForgotPasswordRequest
	if !bindJSON(c, &req) {
		return
	}

	if err := h.authService.ForgotPassword(req); err != nil {
		handleError(c, err)
		return
	}
	response.Success(c, nil)
}

// ResetPassword 重置密码。
func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req dto.ResetPasswordRequest
	if !bindJSON(c, &req) {
		return
	}

	if err := h.authService.ResetPassword(req); err != nil {
		handleError(c, err)
		return
	}
	response.Success(c, nil)
}

// Profile 获取当前用户资料。
func (h *AuthHandler) Profile(c *gin.Context) {
	user, err := h.authService.GetProfile(middleware.CurrentUserID(c))
	if err != nil {
		handleError(c, err)
		return
	}
	response.Success(c, user)
}

// UpdateProfile 更新当前用户资料。
func (h *AuthHandler) UpdateProfile(c *gin.Context) {
	var req dto.UpdateProfileRequest
	if !bindJSON(c, &req) {
		return
	}

	user, err := h.authService.UpdateProfile(middleware.CurrentUserID(c), req)
	if err != nil {
		handleError(c, err)
		return
	}
	response.Success(c, user)
}

// ChangePassword 修改密码。
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	var req dto.ChangePasswordRequest
	if !bindJSON(c, &req) {
		return
	}

	if err := h.authService.ChangePassword(middleware.CurrentUserID(c), req); err != nil {
		handleError(c, err)
		return
	}
	response.Success(c, nil)
}
