package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	ctxRequestID    = "request_id"
	headerRequestID = "X-Request-Id"
	// maxRequestIDLen 限制客户端请求 ID 的长度，避免超长请求头被原样回显到响应头。
	maxRequestIDLen = 64
)

// RequestID 为每个请求注入唯一 ID（复用传入的 X-Request-Id，否则生成），并写回响应头。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 客户端传入的 ID 不可直接采信：非法字符会污染日志行（换行可伪造日志条目），
		// 超长值会被原样写回响应头。格式不合规时改用服务端生成的 UUID。
		rid := sanitizeRequestID(c.GetHeader(headerRequestID))
		if rid == "" {
			rid = uuid.NewString()
		}
		c.Set(ctxRequestID, rid)
		c.Header(headerRequestID, rid)
		c.Next()
	}
}

// sanitizeRequestID 校验客户端请求 ID：仅允许字母、数字与 -_. 且长度不超过上限。
func sanitizeRequestID(rid string) string {
	if rid == "" || len(rid) > maxRequestIDLen {
		return ""
	}
	for _, r := range rid {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
		default:
			return ""
		}
	}
	return rid
}

// CurrentRequestID 从上下文获取当前请求 ID。
func CurrentRequestID(c *gin.Context) string {
	if v, ok := c.Get(ctxRequestID); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
