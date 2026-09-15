package router

import (
	"log/slog"
	"net/http"

	"goroutice/internal/config"
	"goroutice/internal/handler"
	"goroutice/internal/middleware"
	"goroutice/internal/pkg/jwt"
	"goroutice/internal/pkg/response"
	"goroutice/internal/repository"

	"github.com/casbin/casbin/v3"
	"github.com/gin-gonic/gin"
)

// Handlers 汇总所有 HTTP 处理器。
type Handlers struct {
	Auth     *handler.AuthHandler
	User     *handler.UserHandler
	Category *handler.CategoryHandler
	Tag      *handler.TagHandler
	Article  *handler.ArticleHandler
	File     *handler.FileHandler
	Health   *handler.HealthHandler
	Policy   *handler.PolicyHandler
	Updater  *handler.UpdaterHandler
}

// New 构建并配置 Gin 引擎及全部路由。
func New(cfg *config.Config, jwtMgr *jwt.Manager, enforcer *casbin.Enforcer, userRepo *repository.UserRepository, h Handlers, limiter *middleware.RateLimiter) *gin.Engine {
	gin.SetMode(cfg.Server.Mode)

	r := gin.New()
	// 默认不信任任何代理头：Gin 默认信任全部代理，X-Forwarded-For 可被任意伪造，
	// 而限流键依赖 ClientIP，伪造即可绕过分桶限流。前置反向代理时在
	// server.trusted_proxies 里配置代理地址。
	if err := r.SetTrustedProxies(cfg.Server.TrustedProxies); err != nil {
		slog.Error("invalid server.trusted_proxies, proxy headers will not be trusted", "err", err)
		_ = r.SetTrustedProxies(nil)
	}
	// 未匹配路由/方法也返回统一 JSON，避免默认的 text/plain 404 破坏响应契约。
	r.HandleMethodNotAllowed = true
	r.NoRoute(func(c *gin.Context) {
		response.Error(c, http.StatusNotFound, "not found")
	})
	r.NoMethod(func(c *gin.Context) {
		response.Error(c, http.StatusMethodNotAllowed, "method not allowed")
	})
	r.Use(
		middleware.RequestID(),
		middleware.BodyLimit(cfg.Server.MaxBodySize),
		middleware.Logger(),
		middleware.Recovery(),
		middleware.CORS(cfg.CORS.AllowedOrigins),
	)

	// 健康检查
	r.GET("/health/live", h.Health.Liveness)
	r.GET("/health/ready", h.Health.Readiness)

	// 上传文件静态资源
	r.Static("/uploads", cfg.Upload.Path)

	api := r.Group("/api/v1")

	// 认证（无需登录）
	auth := api.Group("/auth")
	auth.POST("/register", limiter.Handler(), h.Auth.Register)
	auth.POST("/login", limiter.Handler(), h.Auth.Login)
	auth.POST("/refresh", limiter.Handler(), h.Auth.Refresh)
	auth.POST("/logout", h.Auth.Logout)
	auth.POST("/verify-email", h.Auth.VerifyEmail)
	auth.POST("/resend-verification", limiter.Handler(), h.Auth.ResendVerification)
	auth.POST("/forgot-password", limiter.Handler(), h.Auth.ForgotPassword)
	auth.POST("/reset-password", limiter.Handler(), h.Auth.ResetPassword)

	authRequired := auth.Group("")
	authRequired.Use(middleware.Auth(jwtMgr, userRepo), middleware.Authorize(enforcer), limiter.WriteHandler("write"))
	{
		authRequired.GET("/profile", h.Auth.Profile)
		authRequired.PUT("/profile", h.Auth.UpdateProfile)
		authRequired.PUT("/password", h.Auth.ChangePassword)
		authRequired.POST("/logout-all", h.Auth.LogoutAll)
		authRequired.DELETE("/me", h.Auth.DeleteAccount)
	}

	// 公开内容
	api.GET("/categories", h.Category.List)
	api.GET("/categories/:id", h.Category.Get)
	api.GET("/tags", h.Tag.List)
	api.GET("/tags/:id", h.Tag.Get)
	api.GET("/articles", h.Article.PublicList)
	api.GET("/articles/:key", h.Article.PublicDetail)

	// 需登录
	authorized := api.Group("")
	authorized.Use(middleware.Auth(jwtMgr, userRepo), middleware.Authorize(enforcer), limiter.WriteHandler("write"))
	{
		authorized.POST("/files", h.File.Upload)
		authorized.GET("/files", h.File.List)
		authorized.DELETE("/files/:id", h.File.Delete)

		authorized.GET("/me/articles", h.Article.MineList)
		authorized.GET("/me/articles/:id", h.Article.MineDetail)
		authorized.POST("/articles", h.Article.Create)
		authorized.PUT("/articles/:id", h.Article.Update)
		authorized.DELETE("/articles/:id", h.Article.Delete)
	}

	// 管理员
	admin := api.Group("/admin")
	admin.Use(middleware.Auth(jwtMgr, userRepo), middleware.Authorize(enforcer), limiter.WriteHandler("write"))
	{
		admin.GET("/users", h.User.List)
		admin.PUT("/users/:id", h.User.Update)
		admin.DELETE("/users/:id", h.User.Delete)

		admin.POST("/categories", h.Category.Create)
		admin.PUT("/categories/:id", h.Category.Update)
		admin.DELETE("/categories/:id", h.Category.Delete)

		admin.POST("/tags", h.Tag.Create)
		admin.PUT("/tags/:id", h.Tag.Update)
		admin.DELETE("/tags/:id", h.Tag.Delete)

		admin.GET("/articles", h.Article.AdminList)
		admin.PUT("/articles/:id/status", h.Article.UpdateStatus)
		admin.PUT("/articles/:id/feature", h.Article.SetFeature)

		// 权限与角色管理
		admin.GET("/policies", h.Policy.List)
		admin.POST("/policies", h.Policy.AddPolicy)
		admin.DELETE("/policies", h.Policy.RemovePolicy)
		admin.POST("/roles", h.Policy.AddRole)
		admin.DELETE("/roles", h.Policy.RemoveRole)
		admin.PUT("/users/:id/role", h.Policy.AssignRole)
		admin.GET("/audits", h.Policy.ListAudits)

		// 自动更新状态（服务端自更新，仅展示与手动检查）
		admin.GET("/updater", h.Updater.Status)
		admin.POST("/updater/check", h.Updater.Check)
	}

	return r
}
