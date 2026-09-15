package middleware

import (
	"net/http"

	"goroutice/internal/pkg/response"

	"github.com/gin-gonic/gin"
)

// BodyLimit 限制请求体大小，防止超大请求耗尽内存。
func BodyLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if maxBytes <= 0 {
			c.Next()
			return
		}
		// 优先用 Content-Length 快速拒绝。
		if c.Request.ContentLength > maxBytes {
			response.Error(c, http.StatusRequestEntityTooLarge, "request body too large")
			c.Abort()
			return
		}
		// 兜底：分块传输等无 Content-Length 时，限制实际读取字节数。
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}
