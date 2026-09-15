package service

import (
	"testing"

	"goroutice/internal/authz"
	"goroutice/internal/model"
	"goroutice/internal/repository"

	"gorm.io/gorm"
)

func newPolicyService(t *testing.T) (*PolicyService, *gorm.DB) {
	t.Helper()

	db := setupTestDB(t)
	e, err := authz.NewEnforcer(db)
	if err != nil {
		t.Fatalf("new enforcer: %v", err)
	}
	if err := authz.SeedPolicies(e); err != nil {
		t.Fatalf("seed policies: %v", err)
	}
	svc := NewPolicyService(e, repository.NewUserRepository(db), repository.NewUserRoleRepository(db), NewAuditService(repository.NewPermissionAuditRepository(db)))
	return svc, db
}

func TestPolicyService_ManagePolicies(t *testing.T) {
	svc, _ := newPolicyService(t)
	op := Operator{ID: "u-admin", Username: "admin"}

	policies, roles, err := svc.ListPolicies()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(policies) == 0 {
		t.Fatal("expected seeded policies")
	}
	if len(roles) == 0 {
		t.Fatal("expected seeded role inheritance")
	}

	// 新增策略
	if err := svc.AddPolicy(op, "admin", "/api/v1/admin/custom", "GET"); err != nil {
		t.Fatalf("add policy: %v", err)
	}
	// 重复新增报冲突
	if err := svc.AddPolicy(op, "admin", "/api/v1/admin/custom", "GET"); err == nil {
		t.Fatal("expected conflict for duplicate policy")
	}
	// 删除
	if err := svc.RemovePolicy(op, "admin", "/api/v1/admin/custom", "GET"); err != nil {
		t.Fatalf("remove policy: %v", err)
	}
	// 删除不存在报 not found
	if err := svc.RemovePolicy(op, "admin", "/api/v1/admin/custom", "GET"); err == nil {
		t.Fatal("expected not found for missing policy")
	}

	// 角色继承
	if err := svc.AddRoleInheritance(op, "editor", "user"); err != nil {
		t.Fatalf("add role: %v", err)
	}
	if err := svc.AddRoleInheritance(op, "editor", "user"); err == nil {
		t.Fatal("expected conflict for duplicate role")
	}
	if err := svc.RemoveRoleInheritance(op, "editor", "user"); err != nil {
		t.Fatalf("remove role: %v", err)
	}
	if err := svc.RemoveRoleInheritance(op, "editor", "user"); err == nil {
		t.Fatal("expected not found for missing role")
	}
}

func TestPolicyService_AssignRole(t *testing.T) {
	svc, db := newPolicyService(t)
	op := Operator{ID: "u-admin", Username: "admin"}
	u := createTestUser(t, db, "alice", model.RoleUser)

	if err := svc.AssignRole(op, u.ID, []string{model.RoleAdmin}); err != nil {
		t.Fatalf("assign role: %v", err)
	}

	got, err := repository.NewUserRepository(db).GetByID(u.ID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if got.Role != model.RoleAdmin {
		t.Fatalf("expected role %q, got %q", model.RoleAdmin, got.Role)
	}

	// 用户不存在
	if err := svc.AssignRole(op, "no-such-id", []string{model.RoleAdmin}); err == nil {
		t.Fatal("expected not found for missing user")
	}
}

func TestPolicyService_ListAudits(t *testing.T) {
	svc, _ := newPolicyService(t)
	op := Operator{ID: "u-admin", Username: "admin"}

	if err := svc.AddPolicy(op, "admin", "/api/v1/admin/custom", "GET"); err != nil {
		t.Fatalf("add policy: %v", err)
	}
	if err := svc.AddRoleInheritance(op, "editor", "user"); err != nil {
		t.Fatalf("add role: %v", err)
	}

	audits, total, err := svc.ListAudits(1, 10, "")
	if err != nil {
		t.Fatalf("list audits: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected 2 audits, got %d", total)
	}
	if len(audits) != 2 {
		t.Fatalf("expected 2 audit items, got %d", len(audits))
	}

	// 关键字过滤
	_, total, err = svc.ListAudits(1, 10, "policy_add")
	if err != nil {
		t.Fatalf("list audits filtered: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected 1 filtered audit, got %d", total)
	}
}

func TestPolicyService_AssignMultipleRoles(t *testing.T) {
	svc, db := newPolicyService(t)
	op := Operator{ID: "u-admin", Username: "admin"}
	u := createTestUser(t, db, "alice", model.RoleUser)

	if err := svc.AssignRole(op, u.ID, []string{model.RoleAuthor, model.RoleUser}); err != nil {
		t.Fatalf("assign roles: %v", err)
	}

	// 主角色取首个，用于向后兼容展示。
	got, err := repository.NewUserRepository(db).GetByID(u.ID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if got.Role != model.RoleAuthor {
		t.Fatalf("expected primary role %q, got %q", model.RoleAuthor, got.Role)
	}

	// 关联表包含全部角色。
	roles, err := repository.NewUserRoleRepository(db).GetRolesByUserID(u.ID)
	if err != nil {
		t.Fatalf("get roles: %v", err)
	}
	if len(roles) != 2 {
		t.Fatalf("expected 2 roles, got %v", roles)
	}
}
