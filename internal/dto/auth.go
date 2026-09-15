package dto

// RegisterRequest 注册请求。
type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=32"`
	Email    string `json:"email" binding:"required,email,max=128"`
	Password string `json:"password" binding:"required,min=8,max=64"`
	Nickname string `json:"nickname" binding:"max=64"`
}

// LoginRequest 登录请求，Account 可为用户名或邮箱。
type LoginRequest struct {
	Account  string `json:"account" binding:"required,max=128"`
	Password string `json:"password" binding:"required,max=64"`
}

// TokenPair access/refresh 令牌对。
type TokenPair struct {
	AccessToken      string `json:"access_token"`
	AccessExpiresAt  string `json:"access_expires_at"`
	RefreshToken     string `json:"refresh_token"`
	RefreshExpiresAt string `json:"refresh_expires_at"`
}

// LoginResponse 登录响应。
type LoginResponse struct {
	Tokens TokenPair `json:"tokens"`
	User   *UserInfo `json:"user"`
}

// RefreshRequest 刷新令牌请求。
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// LogoutRequest 登出请求。
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// VerifyEmailRequest 邮箱验证请求。
type VerifyEmailRequest struct {
	Token string `json:"token" binding:"required"`
}

// ResendVerificationRequest 重新发送验证邮件请求。
type ResendVerificationRequest struct {
	Email string `json:"email" binding:"required,email,max=128"`
}

// ForgotPasswordRequest 忘记密码请求。
type ForgotPasswordRequest struct {
	Email string `json:"email" binding:"required,email,max=128"`
}

// ResetPasswordRequest 重置密码请求。
type ResetPasswordRequest struct {
	Token       string `json:"token" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=8,max=64"`
}

// ChangePasswordRequest 修改密码请求。
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required,max=64"`
	NewPassword string `json:"new_password" binding:"required,min=8,max=64"`
}
