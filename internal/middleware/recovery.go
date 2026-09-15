package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"goroutice/internal/pkg/response"

	"github.com/gin-gonic/gin"
)

// Recovery 捕获 panic，记录结构化日志（含 request_id 与堆栈）并返回 500，避免进程崩溃。
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic recovered",
					"request_id", CurrentRequestID(c),
					"method", c.Request.Method,
					"path", c.Request.URL.Path,
					"panic", fmt.Sprint(r),
					"stack", string(debug.Stack()),
				)
				// 响应可能已被部分写出（如流式输出中途 panic）：此时再写会追加一段
				// 截断的 JSON，反而让客户端拿到畸形响应体。只在未写出时补 500。
				if !c.Writer.Written() {
					response.Error(c, http.StatusInternalServerError, "internal server error")
				}
				c.Abort()
			}
		}()
		c.Next()
	}
}
