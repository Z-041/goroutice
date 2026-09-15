package service

import (
	"testing"

	"goroutice/internal/dto"
	"goroutice/internal/model"
	"goroutice/internal/pkg/jwt"
	"goroutice/internal/repository"
)

func TestAuditService_RecordAndList(t *testing.T) {
	db := setupTestDB(t)
	auditor := NewAuditService(repository.NewPermissionAuditRepository(db))

	auditor.Record(Operator{ID: "u1", Username: "admin"}, model.AuditUserUpdate, "user=bob")

	items, total, err := auditor.List(1, 10, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("total=%d len=%d, want 1", total, len(items))
	}
	if items[0].Action != model.AuditUserUpdate || items[0].OperatorUsername != "admin" {
		t.Fatalf("unexpected item: %+v", items[0])
	}
}

func TestAuditService_NilSafe(t *testing.T) {
	var auditor *AuditService
	auditor.Record(Operator{Username: "ghost"}, model.AuditLogin, "noop")

	items, total, err := auditor.List(1, 10, "")
	if err != nil {
		t.Fatalf("nil auditor list: %v", err)
	}
	if total != 0 || len(items) != 0 {
		t.Fatalf("nil auditor should be a no-op, got total=%d len=%d", total, len(items))
	}
}

func TestAuthService_RecordsAuthAudit(t *testing.T) {
	db := setupTestDB(t)
	userRepo := repository.NewUserRepository(db)
	roleRepo := repository.NewUserRoleRepository(db)
	refreshRepo := repository.NewRefreshTokenRepository(db)
	jwtMgr := jwt.NewManager("test-secret", 1, "test")
	tokenService := NewTokenService(refreshRepo, userRepo, roleRepo, jwtMgr, 720)

	svc := NewAuthService(AuthDeps{
		UserRepo:                 userRepo,
		RoleRepo:                 roleRepo,
		TokenService:             tokenService,
		EmailVerificationRepo:    repository.NewEmailVerificationRepository(db),
		PasswordResetRepo:        repository.NewPasswordResetRepository(db),
		Mailer:                   NewLogMailer(),
		EmailVerificationEnabled: false,
	})
	auditor := NewAuditService(repository.NewPermissionAuditRepository(db))
	svc.SetAuditor(auditor)

	// 账号不存在：应记录 login_failed
	if _, err := svc.Login(dto.LoginRequest{Account: "ghost", Password: "secret123"}); err == nil {
		t.Fatal("expected login failure for unknown account")
	}
	// 注册成功：应记录 register
	if _, err := svc.Register(dto.RegisterRequest{Username: "alice", Email: "alice@example.com", Password: "secret123"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	// 登录成功：应记录 login
	if _, err := svc.Login(dto.LoginRequest{Account: "alice", Password: "secret123"}); err != nil {
		t.Fatalf("login: %v", err)
	}

	items, total, err := auditor.List(1, 20, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 3 {
		t.Fatalf("total=%d, want 3 (%+v)", total, items)
	}

	actions := make(map[string]int, len(items))
	for _, it := range items {
		actions[it.Action]++
	}
	for _, want := range []string{model.AuditRegister, model.AuditLogin, model.AuditLoginFailed} {
		if actions[want] != 1 {
			t.Fatalf("action %q count=%d, want 1 (got %v)", want, actions[want], actions)
		}
	}
}
