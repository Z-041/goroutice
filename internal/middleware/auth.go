package middleware

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"goroutice/internal/model"
	"goroutice/internal/pkg/jwt"
	"goroutice/internal/pkg/response"
	"goroutice/internal/repository"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	ctxUserID   = "user_id"
	ctxUsername = "username"
	ctxRoles    = "roles"
)

// Auth 校验 Bearer Token，并将用户信息写入上下文。
// 同时校验 token 版本与用户状态，使改密/注销/全端下线后的旧令牌立即失效。
func Auth(jwtMgr *jwt.Manager, userRepo *repository.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			response.Error(c, http.StatusUnauthorized, "missing authorization header")
			c.Abort()
			return
		}

		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			response.Error(c, http.StatusUnauthorized, "invalid authorization header")
			c.Abort()
			return
		}

		claims, err := jwtMgr.ParseToken(parts[1])
		if err != nil {
			response.Error(c, http.StatusUnauthorized, "invalid or expired token")
			c.Abort()
			return
		}

		// 校验用户仍存在、处于激活状态，且 token 版本一致（未被主动失效）。
		user, err := userRepo.GetByID(claims.UserID)
		if err != nil {
			// 区分「用户不存在」与「数据库故障」：把后者伪装成 401 会让客户端登出，
			// 也会让故障排查完全跑偏（真实原因只留在数据库侧）。
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				slog.Error("auth: load user", "err", err, "user_id", claims.UserID)
				response.Error(c, http.StatusInternalServerError, "internal server error")
				c.Abort()
				return
			}
			response.Error(c, http.StatusUnauthorized, "invalid or expired token")
			c.Abort()
			return
		}
		if user.Status != model.StatusActive || user.TokenVersion != claims.TokenVersion {
			response.Error(c, http.StatusUnauthorized, "invalid or expired token")
			c.Abort()
			return
		}

		c.Set(ctxUserID, claims.UserID)
		c.Set(ctxUsername, claims.Username)
		c.Set(ctxRoles, claims.Roles)
		c.Next()
	}
}

// CurrentUserID 从上下文获取当前用户 ID。
func CurrentUserID(c *gin.Context) string {
	if v, ok := c.Get(ctxUserID); ok {
		if id, ok := v.(string); ok {
			return id
		}
	}
	return ""
}

// CurrentUsername 从上下文获取当前用户名。
func CurrentUsername(c *gin.Context) string {
	if v, ok := c.Get(ctxUsername); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// CurrentRoles 从上下文获取当前用户的全部角色。
func CurrentRoles(c *gin.Context) []string {
	if v, ok := c.Get(ctxRoles); ok {
		if roles, ok := v.([]string); ok {
			return roles
		}
	}
	return nil
}

// CurrentRole 从上下文获取当前用户的主角色（roles 首个元素）。
func CurrentRole(c *gin.Context) string {
	roles := CurrentRoles(c)
	if len(roles) > 0 {
		return roles[0]
	}
	return ""
}
