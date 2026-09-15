package apperror

import "net/http"

// AppError 携带 HTTP 状态码的业务错误，便于在 handler 层统一映射为响应。
type AppError struct {
	Status  int
	Message string
}

func (e *AppError) Error() string { return e.Message }

// New 创建一个 AppError。
func New(status int, message string) *AppError {
	return &AppError{Status: status, Message: message}
}

// BadRequest 400 参数错误。
func BadRequest(message string) *AppError { return New(http.StatusBadRequest, message) }

// Unauthorized 401 未认证。
func Unauthorized(message string) *AppError { return New(http.StatusUnauthorized, message) }

// Forbidden 403 无权限。
func Forbidden(message string) *AppError { return New(http.StatusForbidden, message) }

// NotFound 404 资源不存在。
func NotFound(message string) *AppError { return New(http.StatusNotFound, message) }

// Conflict 409 资源冲突（如唯一键重复）。
func Conflict(message string) *AppError { return New(http.StatusConflict, message) }

// TooManyRequests 429 请求过于频繁（限流或锁定）。
func TooManyRequests(message string) *AppError { return New(http.StatusTooManyRequests, message) }

// Internal 500 服务内部错误。
func Internal(message string) *AppError { return New(http.StatusInternalServerError, message) }
