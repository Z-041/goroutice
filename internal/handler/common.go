package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"goroutice/internal/middleware"
	"goroutice/internal/pkg/apperror"
	"goroutice/internal/pkg/response"
	"goroutice/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// handleError 将业务错误统一映射为响应，未知错误记录日志后返回 500。
func handleError(c *gin.Context, err error) {
	var appErr *apperror.AppError
	if errors.As(err, &appErr) {
		response.Error(c, appErr.Status, appErr.Message)
		return
	}
	slog.Error("internal error", "err", err)
	response.Error(c, http.StatusInternalServerError, "internal server error")
}

// bindJSON 绑定 JSON 请求体：超过体积上限返回 413，其余解析失败返回 400。
func bindJSON(c *gin.Context, obj any) bool {
	if err := c.ShouldBindJSON(obj); err != nil {
		response.Error(c, httpStatusForBodyError(err), "invalid request body")
		return false
	}
	return true
}

// httpStatusForBodyError 区分「请求体过大」与「请求体格式错误」。
func httpStatusForBodyError(err error) int {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}

// parseID 从路径参数 id 解析 UUID，格式非法时返回 400。
func parseID(c *gin.Context) (string, error) {
	return parseUUIDParam(c, "id")
}

// parseUUIDParam 从指定路径参数解析 UUID，缺失或格式非法时返回 400。
// 非法 ID 早于业务逻辑拦下来：让格式错误的字符串进到 SQL 里查询没有任何意义。
func parseUUIDParam(c *gin.Context, name string) (string, error) {
	value := c.Param(name)
	if value == "" {
		return "", apperror.BadRequest("invalid " + name)
	}
	if _, err := uuid.Parse(value); err != nil {
		return "", apperror.BadRequest("invalid " + name)
	}
	return value, nil
}

// parseIDQuery 读取可选过滤参数中的 UUID：为空表示不过滤，非空但非法返回 400。
func parseIDQuery(c *gin.Context, key string) (string, error) {
	value := c.Query(key)
	if value == "" {
		return "", nil
	}
	if _, err := uuid.Parse(value); err != nil {
		return "", apperror.BadRequest("invalid " + key)
	}
	return value, nil
}

// currentOperator 从上下文提取当前操作者信息。
func currentOperator(c *gin.Context) service.Operator {
	return service.Operator{
		ID:       middleware.CurrentUserID(c),
		Username: middleware.CurrentUsername(c),
	}
}

// auditLog 记录一条审计日志（旁路能力，写失败不影响主流程）。
func auditLog(c *gin.Context, auditor *service.AuditService, action, detail string) {
	auditor.Record(currentOperator(c), action, detail)
}
