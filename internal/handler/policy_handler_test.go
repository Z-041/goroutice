package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"goroutice/internal/authz"
	"goroutice/internal/model"
	"goroutice/internal/repository"
	"goroutice/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newPolicyRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)

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

	if err := db.AutoMigrate(&model.User{}, &model.UserRole{}, &model.PermissionAudit{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	e, err := authz.NewEnforcer(db)
	if err != nil {
		t.Fatalf("new enforcer: %v", err)
	}
	if err := authz.SeedPolicies(e); err != nil {
		t.Fatalf("seed policies: %v", err)
	}

	svc := service.NewPolicyService(e, repository.NewUserRepository(db), repository.NewUserRoleRepository(db), service.NewAuditService(repository.NewPermissionAuditRepository(db)))
	h := NewPolicyHandler(svc)

	r := gin.New()
	r.GET("/policies", h.List)
	r.POST("/policies", h.AddPolicy)
	r.DELETE("/policies", h.RemovePolicy)
	r.POST("/roles", h.AddRole)
	r.DELETE("/roles", h.RemoveRole)
	r.PUT("/users/:id/role", h.AssignRole)
	r.GET("/audits", h.ListAudits)
	return r, db
}

func doJSON(r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestPolicyHandler_List(t *testing.T) {
	r, _ := newPolicyRouter(t)

	w := doJSON(r, http.MethodGet, "/policies", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"policies"`) || !strings.Contains(w.Body.String(), `"roles"`) {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestPolicyHandler_AddRemovePolicy(t *testing.T) {
	r, _ := newPolicyRouter(t)
	body := `{"sub":"admin","obj":"/api/v1/admin/custom","act":"GET"}`

	if w := doJSON(r, http.MethodPost, "/policies", body); w.Code != http.StatusOK {
		t.Fatalf("add status = %d, body=%s", w.Code, w.Body.String())
	}
	// 重复新增 409
	if w := doJSON(r, http.MethodPost, "/policies", body); w.Code != http.StatusConflict {
		t.Fatalf("dup add status = %d, want 409", w.Code)
	}
	// 删除成功
	if w := doJSON(r, http.MethodDelete, "/policies", body); w.Code != http.StatusOK {
		t.Fatalf("remove status = %d, body=%s", w.Code, w.Body.String())
	}
	// 删除不存在 404
	if w := doJSON(r, http.MethodDelete, "/policies", body); w.Code != http.StatusNotFound {
		t.Fatalf("missing remove status = %d, want 404", w.Code)
	}
}

func TestPolicyHandler_AssignRole(t *testing.T) {
	r, db := newPolicyRouter(t)

	u := &model.User{
		Username:     "alice",
		Email:        "alice@test.com",
		PasswordHash: "hashed",
		Role:         model.RoleUser,
		Status:       model.StatusActive,
	}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	w := doJSON(r, http.MethodPut, "/users/"+u.ID+"/role", `{"roles":["admin"]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}

	var got model.User
	if err := db.First(&got, "id = ?", u.ID).Error; err != nil {
		t.Fatalf("get user: %v", err)
	}
	if got.Role != model.RoleAdmin {
		t.Fatalf("expected role admin, got %q", got.Role)
	}

	// 非法角色被绑定校验拒绝
	if w := doJSON(r, http.MethodPut, "/users/"+u.ID+"/role", `{"roles":["superuser"]}`); w.Code != http.StatusBadRequest {
		t.Fatalf("invalid role status = %d, want 400", w.Code)
	}
}

func TestPolicyHandler_ListAudits(t *testing.T) {
	r, _ := newPolicyRouter(t)

	doJSON(r, http.MethodPost, "/policies", `{"sub":"admin","obj":"/api/v1/admin/custom","act":"GET"}`)
	doJSON(r, http.MethodPost, "/roles", `{"child":"editor","parent":"user"}`)

	w := doJSON(r, http.MethodGet, "/audits", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"total":2`) {
		t.Fatalf("expected 2 audits, body=%s", w.Body.String())
	}
}
