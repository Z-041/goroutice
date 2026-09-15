package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"goroutice/internal/authz"
	"goroutice/internal/config"
	"goroutice/internal/database"
	"goroutice/internal/handler"
	"goroutice/internal/middleware"
	"goroutice/internal/pkg/jwt"
	"goroutice/internal/repository"
	"goroutice/internal/router"
	"goroutice/internal/service"
	"goroutice/internal/updater"

	"github.com/casbin/casbin/v3"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// devVersion 表示「本机构建，没有注入版本号」。
const devVersion = "dev"

// version 是当前二进制版本，唯一源头为 git tag，由 CI 通过 -ldflags 注入：
//
//	go build -ldflags "-X main.version=v1.2.3" -o bin/server ./cmd/server
var version = devVersion

// shortRevisionLen 是回退版本号里保留的提交哈希长度，与 git 默认缩写一致。
const shortRevisionLen = 7

// resolveVersion 返回用于展示的版本号：优先用注入的 tag，没有时回退到编译器写入的
// VCS 信息，拼成 dev+<短提交>（工作区有未提交改动时追加 -dirty）。
//
// 之所以要回退：本地 go build / go run 不会注入版本，管理端「当前版本」就只剩一个
// 无法追溯的 dev——线上出问题时，连是哪个提交构建的都查不到。
// 回退值刻意保持非语义化，不用 0.0.0-<commit>：后者会被自动更新当成比任何发布版都旧的
// 版本，从而把本地构建悄悄替换成发布版。非语义化版本会让自动更新自动停用。
func resolveVersion() string {
	if version != devVersion {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return devVersion
	}
	var revision string
	var dirty bool
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			dirty = setting.Value == "true"
		}
	}
	if revision == "" {
		// 构建时关了 VCS 采集（-buildvcs=false）或在仓库外构建，没有可回退的信息。
		return devVersion
	}
	if len(revision) > shortRevisionLen {
		revision = revision[:shortRevisionLen]
	}
	if dirty {
		return devVersion + "+" + revision + "-dirty"
	}
	return devVersion + "+" + revision
}

func main() {
	version = resolveVersion()
	cfg := loadConfig()
	db, fulltext := initDatabase(cfg)
	enforcer := initEnforcer(db)
	jwtMgr := jwt.NewManager(cfg.JWT.Secret, cfg.JWT.AccessExpireMinutes, cfg.JWT.Issuer)

	// 监听系统信号，实现优雅关闭。
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 后台自动更新：定时发现新版本，校验通过后替换二进制并重启自身。
	// 返回的实例同时供管理端查询状态与手动触发检查；未启用时为 nil。
	autoUpdater := startAutoUpdater(ctx, cfg)
	application := newApp(cfg, db, enforcer, jwtMgr, autoUpdater, fulltext)
	application.startBackground(ctx)

	engine := router.New(cfg, jwtMgr, enforcer, application.userRepo, application.handlers, application.limiter)
	serve(ctx, stop, cfg, engine, db)
}

// app 汇总路由与后台任务所需的装配结果。
type app struct {
	userRepo              *repository.UserRepository
	refreshTokenRepo      *repository.RefreshTokenRepository
	emailVerificationRepo *repository.EmailVerificationRepository
	passwordResetRepo     *repository.PasswordResetRepository
	handlers              router.Handlers
	limiter               *middleware.RateLimiter
	loginLimiter          *service.LoginLimiter
	viewTracker           *service.ViewTracker
}

// tokenCleanupInterval 是过期令牌的清理周期。
const tokenCleanupInterval = time.Hour

// loadConfig 加载配置文件并校验 JWT 密钥强度，失败时终止启动。
func loadConfig() *config.Config {
	cfg, err := config.Load("config.yaml")
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}
	if len(cfg.JWT.Secret) < 16 {
		// 空密钥等同于「任何人都能伪造令牌」，无论在哪种模式下都不允许启动。
		if cfg.JWT.Secret == "" || cfg.Server.Mode == "release" {
			slog.Error("refusing to start: jwt.secret is missing or too weak (at least 16 chars)")
			os.Exit(1)
		}
		slog.Warn("jwt.secret is weak; set a strong secret in production")
	}
	return cfg
}

// initDatabase 连接数据库、执行自动迁移、确保全文索引可用并初始化管理员账号，失败时终止启动。
// 返回的第二个值表示文章全文索引是否可用，用于决定关键词检索走全文检索还是 LIKE 回退。
func initDatabase(cfg *config.Config) (*gorm.DB, bool) {
	db, err := database.NewMySQL(&cfg.Database)
	if err != nil {
		slog.Error("connect database", "err", err)
		os.Exit(1)
	}
	if err := database.AutoMigrate(db); err != nil {
		slog.Error("auto migrate", "err", err)
		os.Exit(1)
	}
	// 建索引失败只降级不中断：检索会退回 LIKE，慢但结果正确。
	fulltext := database.EnsureArticleFulltextIndex(db)
	if err := database.SeedAdmin(db, cfg.Bootstrap.AdminUsername, cfg.Bootstrap.AdminPassword, cfg.Bootstrap.AdminEmail); err != nil {
		slog.Error("seed admin", "err", err)
		os.Exit(1)
	}
	return db, fulltext
}

// initEnforcer 初始化权限执行器并写入种子策略，失败时终止启动。
func initEnforcer(db *gorm.DB) *casbin.Enforcer {
	enforcer, err := authz.NewEnforcer(db)
	if err != nil {
		slog.Error("init casbin", "err", err)
		os.Exit(1)
	}
	if err := authz.SeedPolicies(enforcer); err != nil {
		slog.Error("seed casbin policies", "err", err)
		os.Exit(1)
	}
	return enforcer
}

// newApp 装配数据访问层、业务逻辑层、处理器与限流器。fulltext 表示文章全文索引是否可用。
func newApp(cfg *config.Config, db *gorm.DB, enforcer *casbin.Enforcer, jwtMgr *jwt.Manager, autoUpdater *updater.Updater, fulltext bool) *app {
	// 数据访问层
	userRepo := repository.NewUserRepository(db)
	roleRepo := repository.NewUserRoleRepository(db)
	refreshRepo := repository.NewRefreshTokenRepository(db)
	emailVerificationRepo := repository.NewEmailVerificationRepository(db)
	passwordResetRepo := repository.NewPasswordResetRepository(db)
	categoryRepo := repository.NewCategoryRepository(db)
	tagRepo := repository.NewTagRepository(db)
	articleRepo := repository.NewArticleRepository(db, fulltext)
	articleRevisionRepo := repository.NewArticleRevisionRepository(db)
	fileRepo := repository.NewFileRepository(db)
	auditRepo := repository.NewPermissionAuditRepository(db)

	// 业务逻辑层
	auditService := service.NewAuditService(auditRepo)
	loginLimiter := service.NewLoginLimiter(cfg.Security.MaxLoginAttempts, cfg.Security.LoginLockMinutes)
	tokenService := service.NewTokenService(refreshRepo, userRepo, roleRepo, jwtMgr, cfg.JWT.RefreshExpireHours)
	mailer := newMailer(cfg)
	authService := service.NewAuthService(service.AuthDeps{
		UserRepo:                 userRepo,
		RoleRepo:                 roleRepo,
		TokenService:             tokenService,
		EmailVerificationRepo:    emailVerificationRepo,
		PasswordResetRepo:        passwordResetRepo,
		Mailer:                   mailer,
		EmailVerificationEnabled: cfg.Security.EmailVerificationEnabled,
	})
	authService.SetLoginLimiter(loginLimiter)
	authService.SetPasswordPolicy(service.PasswordPolicy{
		MinLength:        cfg.Security.PasswordMinLength,
		RequireUppercase: cfg.Security.PasswordRequireUppercase,
		RequireSpecial:   cfg.Security.PasswordRequireSpecial,
	})
	authService.SetAuditor(auditService)

	userService := service.NewUserService(userRepo, roleRepo)
	categoryService := service.NewCategoryService(categoryRepo)
	tagService := service.NewTagService(tagRepo)
	// 浏览量去重器：按 来源 + 文章 在窗口内只计一次浏览。
	viewTracker := service.NewViewTracker(cfg.Article.ViewDedupMinutes)
	articleService := service.NewArticleService(
		articleRepo, categoryRepo, tagRepo, articleRevisionRepo,
		cfg.Article.RevisionKeep,
		viewTracker,
	)
	fileService := service.NewFileService(fileRepo, &cfg.Upload)
	policyService := service.NewPolicyService(enforcer, userRepo, roleRepo, auditService)

	// 接口限流器
	limiter := middleware.NewRateLimiter(cfg.RateLimit.RPS, cfg.RateLimit.Burst)
	limiter.SetTier("write", cfg.RateLimit.WriteRPS, cfg.RateLimit.WriteBurst)

	return &app{
		userRepo:              userRepo,
		refreshTokenRepo:      refreshRepo,
		emailVerificationRepo: emailVerificationRepo,
		passwordResetRepo:     passwordResetRepo,
		handlers: router.Handlers{
			Auth:     handler.NewAuthHandler(authService),
			User:     handler.NewUserHandler(userService, auditService),
			Category: handler.NewCategoryHandler(categoryService, auditService),
			Tag:      handler.NewTagHandler(tagService, auditService),
			Article:  handler.NewArticleHandler(articleService, auditService),
			File:     handler.NewFileHandler(fileService, auditService),
			Health:   handler.NewHealthHandler(db),
			Policy:   handler.NewPolicyHandler(policyService),
			Updater:  handler.NewUpdaterHandler(autoUpdater, version),
			Site:     handler.NewSiteHandler(articleService, cfg.Site),
		},
		limiter:      limiter,
		loginLimiter: loginLimiter,
		viewTracker:  viewTracker,
	}
}

// startBackground 启动依赖 ctx 的周期清理任务。
func (a *app) startBackground(ctx context.Context) {
	go a.limiter.Cleanup(ctx, time.Minute)
	go a.loginLimiter.Cleanup(ctx, time.Minute)
	go a.viewTracker.Cleanup(ctx, time.Minute)
	go a.cleanupExpiredTokens(ctx)
}

// cleanupExpiredTokens 周期性删除三张令牌表中的过期记录。
// 这些表只增不减：不做清理会让刷新/验证/重置令牌随运行时间无限堆积。
func (a *app) cleanupExpiredTokens(ctx context.Context) {
	ticker := time.NewTicker(tokenCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			cleanups := []struct {
				table string
				run   func(time.Time) error
			}{
				{"refresh_token", a.refreshTokenRepo.DeleteExpired},
				{"email_verification", a.emailVerificationRepo.DeleteExpired},
				{"password_reset", a.passwordResetRepo.DeleteExpired},
			}
			for _, c := range cleanups {
				if err := c.run(now); err != nil {
					slog.Error("cleanup expired token", "table", c.table, "err", err)
				}
			}
		}
	}
}

// serve 启动 HTTP 服务并阻塞至收到退出信号，随后优雅关闭并释放数据库连接。
func serve(ctx context.Context, stop context.CancelFunc, cfg *config.Config, engine *gin.Engine, db *gorm.DB) {
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      engine,
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(cfg.Server.IdleTimeout) * time.Second,
	}

	go func() {
		slog.Info("server starting", "addr", srv.Addr, "mode", cfg.Server.Mode, "version", version)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down server")

	// 父 ctx 此时已被取消，需从它派生一个不受取消影响的关闭超时窗口。
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown failed", "err", err)
	}

	// 关闭数据库连接池，释放资源。
	if sqlDB, err := db.DB(); err == nil {
		if err := sqlDB.Close(); err != nil {
			slog.Error("close database", "err", err)
		}
	}
	slog.Info("server stopped")
}

// newMailer 根据配置构造邮件发送实现：启用 SMTP 时使用真实发信，否则回退到日志邮件。
func newMailer(cfg *config.Config) service.Mailer {
	if cfg.SMTP.Enabled {
		if cfg.SMTP.Username == "" || cfg.SMTP.Password == "" {
			// 凭据缺失时登录必然失败，提前把原因说清楚，避免只在注册/找回密码时才暴露。
			slog.Warn("smtp credentials are empty; sending mail will fail",
				"username_set", cfg.SMTP.Username != "", "password_set", cfg.SMTP.Password != "")
		}
		slog.Info("using smtp mailer", "host", cfg.SMTP.Host, "port", cfg.SMTP.Port)
		return service.NewSmtpMailer(cfg.SMTP)
	}
	slog.Info("using log mailer")
	return service.NewLogMailer()
}

// startAutoUpdater 在配置开启时启动后台自动更新轮询并返回更新器实例（未启用时返回 nil），
// 配置缺失时直接终止启动。
func startAutoUpdater(ctx context.Context, cfg *config.Config) *updater.Updater {
	if !cfg.Updater.Enabled {
		return nil
	}
	autoUpdater, err := newUpdater(cfg)
	if err != nil {
		slog.Error("init updater", "err", err)
		os.Exit(1)
	}
	go autoUpdater.Run(ctx)
	return autoUpdater
}

// newUpdater 根据配置构造自动更新器；主源为 GitHub，备用源为 Gitee，版本号来自构建时注入的 main.version。
func newUpdater(cfg *config.Config) (*updater.Updater, error) {
	updaterCfg := cfg.Updater
	sources := make([]updater.Source, 0, 2)
	if updaterCfg.GitHubOwner != "" && updaterCfg.GitHubRepo != "" {
		sources = append(sources, updater.GitHubSource(updaterCfg.GitHubOwner, updaterCfg.GitHubRepo, updaterCfg.GitHubToken))
	}
	if updaterCfg.GiteeOwner != "" && updaterCfg.GiteeRepo != "" {
		sources = append(sources, updater.GiteeSource(updaterCfg.GiteeOwner, updaterCfg.GiteeRepo, updaterCfg.GiteeToken))
	}
	return updater.New(updater.Config{
		CurrentVersion: version,
		Sources:        sources,
		Interval:       time.Duration(updaterCfg.IntervalMinutes) * time.Minute,
	})
}
