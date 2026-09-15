package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"goroutice/internal/authz"
	"goroutice/internal/config"
	"goroutice/internal/database"
	"goroutice/internal/handler"
	"goroutice/internal/middleware"
	"goroutice/internal/model"
	"goroutice/internal/pkg/jwt"
	"goroutice/internal/repository"
	"goroutice/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// testEnv 集成测试环境，包含完整路由、JWT 管理器及三种角色用户的 ID。
type testEnv struct {
	engine   *gin.Engine
	mgr      *jwt.Manager
	userID   string
	authorID string
	adminID  string
}

// newTestEnv 构建完整路由与依赖（内存 SQLite + Casbin + JWT），并预置三种角色用户。
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	if err := database.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	enforcer, err := authz.NewEnforcer(db)
	if err != nil {
		t.Fatalf("new enforcer: %v", err)
	}
	if err := authz.SeedPolicies(enforcer); err != nil {
		t.Fatalf("seed policies: %v", err)
	}

	userID := createTestUser(t, db, "alice", model.RoleUser)
	authorID := createTestUser(t, db, "bob", model.RoleAuthor)
	adminID := createTestUser(t, db, "carol", model.RoleAdmin)

	jwtMgr := jwt.NewManager("integration-test-secret", 1, "test")

	userRepo := repository.NewUserRepository(db)
	roleRepo := repository.NewUserRoleRepository(db)
	refreshRepo := repository.NewRefreshTokenRepository(db)
	emailVerificationRepo := repository.NewEmailVerificationRepository(db)
	passwordResetRepo := repository.NewPasswordResetRepository(db)
	categoryRepo := repository.NewCategoryRepository(db)
	tagRepo := repository.NewTagRepository(db)
	articleRepo := repository.NewArticleRepository(db)
	fileRepo := repository.NewFileRepository(db)
	auditRepo := repository.NewPermissionAuditRepository(db)

	tokenService := service.NewTokenService(refreshRepo, userRepo, roleRepo, jwtMgr, 720)
	authService := service.NewAuthService(service.AuthDeps{
		UserRepo:                 userRepo,
		RoleRepo:                 roleRepo,
		TokenService:             tokenService,
		EmailVerificationRepo:    emailVerificationRepo,
		PasswordResetRepo:        passwordResetRepo,
		Mailer:                   service.NewLogMailer(),
		EmailVerificationEnabled: false,
	})
	authService.SetLoginLimiter(service.NewLoginLimiter(100, 100))

	auditService := service.NewAuditService(auditRepo)
	authService.SetAuditor(auditService)

	handlers := Handlers{
		Auth:     handler.NewAuthHandler(authService),
		User:     handler.NewUserHandler(service.NewUserService(userRepo, roleRepo), auditService),
		Category: handler.NewCategoryHandler(service.NewCategoryService(categoryRepo), auditService),
		Tag:      handler.NewTagHandler(service.NewTagService(tagRepo), auditService),
		Article:  handler.NewArticleHandler(service.NewArticleService(articleRepo, categoryRepo, tagRepo), auditService),
		File:     handler.NewFileHandler(service.NewFileService(fileRepo, &config.UploadConfig{Path: t.TempDir(), MaxSize: 1 << 20, AllowedExts: []string{".txt"}}), auditService),
		Health:   handler.NewHealthHandler(db),
		Policy:   handler.NewPolicyHandler(service.NewPolicyService(enforcer, userRepo, roleRepo, auditService)),
		Updater:  handler.NewUpdaterHandler(nil, "test"),
	}

	limiter := middleware.NewRateLimiter(1000, 1000)

	cfg := &config.Config{
		Server: config.ServerConfig{Mode: gin.TestMode, MaxBodySize: 1 << 20},
		CORS:   config.CORSConfig{AllowedOrigins: []string{"*"}},
		Upload: config.UploadConfig{Path: t.TempDir()},
	}

	return &testEnv{
		engine:   New(cfg, jwtMgr, enforcer, userRepo, handlers, limiter),
		mgr:      jwtMgr,
		userID:   userID,
		authorID: authorID,
		adminID:  adminID,
	}
}

func createTestUser(t *testing.T, db *gorm.DB, username, role string) string {
	t.Helper()
	u := &model.User{
		Username:     username,
		Email:        username + "@test.com",
		PasswordHash: "hashed",
		Role:         role,
		Status:       model.StatusActive,
	}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("create user %s: %v", username, err)
	}
	if err := db.Create(&model.UserRole{UserID: u.ID, Role: role}).Error; err != nil {
		t.Fatalf("create role for %s: %v", username, err)
	}
	return u.ID
}

func newToken(t *testing.T, mgr *jwt.Manager, uid, name, role string) string {
	t.Helper()
	tok, _, err := mgr.GenerateAccessToken(uid, name, []string{role}, 0)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return tok
}

func doRequest(engine *gin.Engine, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func TestRouter_AuthRequired(t *testing.T) {
	env := newTestEnv(t)

	if w := doRequest(env.engine, http.MethodGet, "/api/v1/admin/users", "", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("no token: got %d, want 401", w.Code)
	}
	if w := doRequest(env.engine, http.MethodGet, "/api/v1/admin/users", "invalid-token", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("invalid token: got %d, want 401", w.Code)
	}
	if w := doRequest(env.engine, http.MethodGet, "/api/v1/categories", "", ""); w.Code != http.StatusOK {
		t.Fatalf("public categories: got %d, want 200", w.Code)
	}
}

func TestRouter_AuthorizationByRole(t *testing.T) {
	env := newTestEnv(t)

	userTok := newToken(t, env.mgr, env.userID, "alice", model.RoleUser)
	authorTok := newToken(t, env.mgr, env.authorID, "bob", model.RoleAuthor)
	adminTok := newToken(t, env.mgr, env.adminID, "carol", model.RoleAdmin)

	cases := []struct {
		name   string
		token  string
		method string
		path   string
		want   int
	}{
		{"user denied admin", userTok, http.MethodGet, "/api/v1/admin/users", http.StatusForbidden},
		{"author denied admin", authorTok, http.MethodGet, "/api/v1/admin/users", http.StatusForbidden},
		{"admin allowed admin", adminTok, http.MethodGet, "/api/v1/admin/users", http.StatusOK},
		{"user denied create article", userTok, http.MethodPost, "/api/v1/articles", http.StatusForbidden},
		{"author allowed create article", authorTok, http.MethodPost, "/api/v1/articles", http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := doRequest(env.engine, tc.method, tc.path, tc.token, "{}")
			if w.Code != tc.want {
				t.Fatalf("got %d, want %d (body=%s)", w.Code, tc.want, w.Body.String())
			}
		})
	}
}

func TestRouter_RoleInheritance(t *testing.T) {
	env := newTestEnv(t)

	userTok := newToken(t, env.mgr, env.userID, "alice", model.RoleUser)
	authorTok := newToken(t, env.mgr, env.authorID, "bob", model.RoleAuthor)
	adminTok := newToken(t, env.mgr, env.adminID, "carol", model.RoleAdmin)

	cases := []struct {
		name  string
		token string
		path  string
		want  int
	}{
		{"user profile", userTok, "/api/v1/auth/profile", http.StatusOK},
		{"author inherits user profile", authorTok, "/api/v1/auth/profile", http.StatusOK},
		{"admin inherits author articles", adminTok, "/api/v1/me/articles", http.StatusOK},
		{"author own articles", authorTok, "/api/v1/me/articles", http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := doRequest(env.engine, http.MethodGet, tc.path, tc.token, "")
			if w.Code != tc.want {
				t.Fatalf("got %d, want %d (body=%s)", w.Code, tc.want, w.Body.String())
			}
		})
	}
}

func TestRouter_MultipleRoles(t *testing.T) {
	env := newTestEnv(t)

	// 用户同时拥有 user + author，应能访问作者文章端点。
	authorTok, _, err := env.mgr.GenerateAccessToken(env.userID, "alice", []string{model.RoleUser, model.RoleAuthor}, 0)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	if w := doRequest(env.engine, http.MethodGet, "/api/v1/me/articles", authorTok, ""); w.Code != http.StatusOK {
		t.Fatalf("multi-role author access: got %d, want 200", w.Code)
	}

	// 用户同时拥有 user + admin，应能访问管理端点。
	adminTok, _, err := env.mgr.GenerateAccessToken(env.userID, "alice", []string{model.RoleUser, model.RoleAdmin}, 0)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}
	if w := doRequest(env.engine, http.MethodGet, "/api/v1/admin/users", adminTok, ""); w.Code != http.StatusOK {
		t.Fatalf("multi-role admin access: got %d, want 200", w.Code)
	}
}
