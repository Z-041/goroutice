package middleware

import (
	"net/http"

	"goroutice/internal/pkg/response"

	"github.com/casbin/casbin/v3"
	"github.com/gin-gonic/gin"
)

// Authorize 使用 Casbin 对当前请求做角色路由授权（sub=角色, obj=路径, act=方法）。
// 依赖 Auth 中间件已将用户角色写入上下文。
func Authorize(enforcer *casbin.Enforcer) gin.HandlerFunc {
	return func(c *gin.Context) {
		obj := c.Request.URL.Path
		act := c.Request.Method

		for _, role := range CurrentRoles(c) {
			ok, err := enforcer.Enforce(role, obj, act)
			if err != nil {
				response.Error(c, http.StatusInternalServerError, "authorization error")
				c.Abort()
				return
			}
			if ok {
				c.Next()
				return
			}
		}

		response.Error(c, http.StatusForbidden, "forbidden")
		c.Abort()
	}
}
