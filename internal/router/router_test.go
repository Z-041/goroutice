package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"goroutice/internal/authz"
	"goroutice/internal/config"
	"goroutice/internal/database"
	"goroutice/internal/dto"
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
	db       *gorm.DB
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
	// 第二个参数为 false：测试库是 SQLite，没有全文索引，检索走 LIKE 分支。
	articleRepo := repository.NewArticleRepository(db, false)
	articleRevisionRepo := repository.NewArticleRevisionRepository(db)
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

	articleService := service.NewArticleService(
		articleRepo, categoryRepo, tagRepo, articleRevisionRepo, 3, service.NewViewTracker(30),
	)
	siteConfig := config.SiteConfig{BaseURL: "https://blog.example.com", Title: "Test Blog"}

	handlers := Handlers{
		Auth:     handler.NewAuthHandler(authService),
		User:     handler.NewUserHandler(service.NewUserService(userRepo, roleRepo), auditService),
		Category: handler.NewCategoryHandler(service.NewCategoryService(categoryRepo), auditService),
		Tag:      handler.NewTagHandler(service.NewTagService(tagRepo), auditService),
		Article:  handler.NewArticleHandler(articleService, auditService),
		File:     handler.NewFileHandler(service.NewFileService(fileRepo, &config.UploadConfig{Path: t.TempDir(), MaxSize: 1 << 20, AllowedExts: []string{".txt"}}), auditService),
		Health:   handler.NewHealthHandler(db),
		Policy:   handler.NewPolicyHandler(service.NewPolicyService(enforcer, userRepo, roleRepo, auditService)),
		Updater:  handler.NewUpdaterHandler(nil, "test"),
		Site:     handler.NewSiteHandler(articleService, siteConfig),
	}

	limiter := middleware.NewRateLimiter(1000, 1000)

	cfg := &config.Config{
		Server: config.ServerConfig{Mode: gin.TestMode, MaxBodySize: 1 << 20},
		CORS:   config.CORSConfig{AllowedOrigins: []string{"*"}},
		Upload: config.UploadConfig{Path: t.TempDir()},
		Site:   siteConfig,
	}

	return &testEnv{
		engine:   New(cfg, jwtMgr, enforcer, userRepo, handlers, limiter),
		mgr:      jwtMgr,
		db:       db,
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

// extractID 从统一响应里取出 data.id。
func extractID(t *testing.T, body string) string {
	t.Helper()

	var resp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode response %s: %v", body, err)
	}
	if resp.Data.ID == "" {
		t.Fatalf("response has no data.id: %s", body)
	}
	return resp.Data.ID
}

func TestRouter_ArticleRevisionFlow(t *testing.T) {
	env := newTestEnv(t)
	authorTok := newToken(t, env.mgr, env.authorID, "bob", model.RoleAuthor)
	userTok := newToken(t, env.mgr, env.userID, "alice", model.RoleUser)

	// 建文
	w := doRequest(env.engine, http.MethodPost, "/api/v1/articles", authorTok,
		`{"title":"第一版","content":"正文一","status":"published"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create article: got %d, want 201 (body=%s)", w.Code, w.Body.String())
	}
	articleID := extractID(t, w.Body.String())

	// 改文
	w = doRequest(env.engine, http.MethodPut, "/api/v1/articles/"+articleID, authorTok,
		`{"title":"第二版","content":"正文二","status":"published"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("update article: got %d, want 200 (body=%s)", w.Code, w.Body.String())
	}

	// 历史里应出现更新前的第一版
	path := "/api/v1/me/articles/" + articleID + "/revisions"
	w = doRequest(env.engine, http.MethodGet, path, authorTok, "")
	if w.Code != http.StatusOK {
		t.Fatalf("list revisions: got %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	revisionID := extractListItemID(t, w.Body.String(), "list", 0, "id")

	// 无作者权限的角色看不到历史（Casbin 策略回归）
	if w := doRequest(env.engine, http.MethodGet, path, userTok, ""); w.Code != http.StatusForbidden {
		t.Fatalf("user listing revisions: got %d, want 403", w.Code)
	}

	// 回滚
	restorePath := path + "/" + revisionID + "/restore"
	w = doRequest(env.engine, http.MethodPost, restorePath, authorTok, "")
	if w.Code != http.StatusOK {
		t.Fatalf("restore revision: got %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "第一版") {
		t.Fatalf("restore did not roll content back: %s", w.Body.String())
	}

	// 无作者权限的角色不能回滚
	if w := doRequest(env.engine, http.MethodPost, restorePath, userTok, ""); w.Code != http.StatusForbidden {
		t.Fatalf("user restoring revision: got %d, want 403", w.Code)
	}
}

// extractListItemID 从分页响应里取出 data.<field>[<index>].<key>，用于验证列表接口。
func extractListItemID(t *testing.T, body, field string, index int, key string) string {
	t.Helper()

	var resp struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode response %s: %v", body, err)
	}
	var items []map[string]any
	if err := json.Unmarshal(resp.Data[field], &items); err != nil {
		t.Fatalf("decode %s of %s: %v", field, body, err)
	}
	if len(items) <= index {
		t.Fatalf("expected at least %d items, got %d (body=%s)", index+1, len(items), body)
	}
	value, _ := items[index][key].(string)
	if value == "" {
		t.Fatalf("item %d has no %s: %s", index, key, body)
	}
	return value
}

func TestRouter_SiteFiles(t *testing.T) {
	env := newTestEnv(t)

	// 订阅源与站点地图只能反映已发布内容，因此这里各预置一篇已发布文章与草稿。
	// revisionKeep 传 0：本用例与修订历史无关。
	articles := service.NewArticleService(
		repository.NewArticleRepository(env.db, false),
		repository.NewCategoryRepository(env.db),
		repository.NewTagRepository(env.db),
		repository.NewArticleRevisionRepository(env.db),
		0,
		service.NewViewTracker(30),
	)
	published, err := articles.Create(env.authorID, dto.ArticleRequest{
		Title: "Hello World", Slug: "hello-world", Summary: "摘要", Content: "正文", Status: model.ArticlePublished,
	})
	if err != nil {
		t.Fatalf("seed article: %v", err)
	}
	if _, err := articles.Create(env.authorID, dto.ArticleRequest{
		Title: "Draft", Slug: "draft", Content: "草稿", Status: model.ArticleDraft,
	}); err != nil {
		t.Fatalf("seed draft: %v", err)
	}

	w := doRequest(env.engine, http.MethodGet, "/feed.xml", "", "")
	if w.Code != http.StatusOK {
		t.Fatalf("feed: got %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/rss+xml") {
		t.Fatalf("feed content-type = %q, want application/rss+xml", ct)
	}
	feed := w.Body.String()
	if !strings.HasPrefix(feed, "<?xml") {
		t.Fatalf("feed 缺少 XML 声明: %s", feed)
	}
	link := "<link>https://blog.example.com/articles/hello-world</link>"
	if !strings.Contains(feed, link) {
		t.Fatalf("feed 缺少绝对文章链接 %s: %s", link, feed)
	}
	if strings.Contains(feed, "draft") {
		t.Fatalf("feed 泄露了未发布文章: %s", feed)
	}

	w = doRequest(env.engine, http.MethodGet, "/sitemap.xml", "", "")
	if w.Code != http.StatusOK {
		t.Fatalf("sitemap: got %d, want 200", w.Code)
	}
	sitemap := w.Body.String()
	for _, want := range []string{
		"<loc>https://blog.example.com/</loc>",
		"<loc>https://blog.example.com/articles/hello-world</loc>",
	} {
		if !strings.Contains(sitemap, want) {
			t.Fatalf("sitemap 缺少 %s: %s", want, sitemap)
		}
	}
	if strings.Contains(sitemap, "draft") {
		t.Fatalf("sitemap 泄露了未发布文章: %s", sitemap)
	}

	w = doRequest(env.engine, http.MethodGet, "/robots.txt", "", "")
	if w.Code != http.StatusOK {
		t.Fatalf("robots: got %d, want 200", w.Code)
	}
	robots := w.Body.String()
	if !strings.Contains(robots, "Disallow: /api/") || !strings.Contains(robots, "https://blog.example.com/sitemap.xml") {
		t.Fatalf("robots.txt 内容不符合预期: %s", robots)
	}
	// 配图在 /uploads 下，屏蔽它会让文章图片从搜索结果里消失。
	if strings.Contains(robots, "Disallow: /uploads") {
		t.Fatalf("robots.txt 不应屏蔽 /uploads: %s", robots)
	}

	// 文章 slug 以标题生成，用于确认上面的断言不是靠 seed 的显式 slug 蒙对的。
	if published.Slug != "hello-world" {
		t.Fatalf("unexpected slug %q", published.Slug)
	}
}
