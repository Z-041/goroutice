package service

import (
	"errors"
	"testing"

	"goroutice/internal/dto"
	"goroutice/internal/model"
	"goroutice/internal/pkg/apperror"
	"goroutice/internal/pkg/jwt"
	"goroutice/internal/repository"

	"gorm.io/gorm"
)

func newAuthService(t *testing.T) (*AuthService, *repository.UserRepository) {
	t.Helper()
	db := setupTestDB(t)
	userRepo := repository.NewUserRepository(db)
	roleRepo := repository.NewUserRoleRepository(db)
	refreshRepo := repository.NewRefreshTokenRepository(db)
	emailVerificationRepo := repository.NewEmailVerificationRepository(db)
	passwordResetRepo := repository.NewPasswordResetRepository(db)
	jwtMgr := jwt.NewManager("test-secret", 1, "test")
	tokenService := NewTokenService(refreshRepo, userRepo, roleRepo, jwtMgr, 720)
	svc := NewAuthService(AuthDeps{
		UserRepo:                 userRepo,
		RoleRepo:                 roleRepo,
		TokenService:             tokenService,
		EmailVerificationRepo:    emailVerificationRepo,
		PasswordResetRepo:        passwordResetRepo,
		Mailer:                   NewLogMailer(),
		EmailVerificationEnabled: false,
	})
	return svc, userRepo
}

func TestAuthService_Register(t *testing.T) {
	svc, _ := newAuthService(t)

	user, err := svc.Register(dto.RegisterRequest{Username: "alice", Email: "alice@example.com", Password: "secret123"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if user.ID == "" {
		t.Fatal("expected non-empty id")
	}
	if user.Username != "alice" || user.Role != model.RoleUser || user.Status != model.StatusActive {
		t.Fatalf("unexpected user: %+v", user)
	}
	if user.Nickname == "" {
		t.Fatal("expected default nickname")
	}

	// 重复用户名
	if _, err := svc.Register(dto.RegisterRequest{Username: "alice", Email: "bob@example.com", Password: "secret123"}); err == nil {
		t.Fatal("expected duplicate username error")
	} else if err != nil {
		var appErr *apperror.AppError
		if !errors.As(err, &appErr) {
			t.Fatalf("expected AppError, got %T: %v", err, err)
		}
	}

	// 重复邮箱
	if _, err := svc.Register(dto.RegisterRequest{Username: "bob", Email: "alice@example.com", Password: "secret123"}); err == nil {
		t.Fatal("expected duplicate email error")
	}
}

func TestAuthService_Login(t *testing.T) {
	svc, _ := newAuthService(t)
	if _, err := svc.Register(dto.RegisterRequest{Username: "alice", Email: "alice@example.com", Password: "secret123"}); err != nil {
		t.Fatalf("register: %v", err)
	}

	// 用户名登录
	resp, err := svc.Login(dto.LoginRequest{Account: "alice", Password: "secret123"})
	if err != nil {
		t.Fatalf("login by username: %v", err)
	}
	if resp.Tokens.AccessToken == "" || resp.User == nil || resp.User.Username != "alice" {
		t.Fatalf("unexpected login response: %+v", resp)
	}

	// 邮箱登录
	if _, err := svc.Login(dto.LoginRequest{Account: "alice@example.com", Password: "secret123"}); err != nil {
		t.Fatalf("login by email: %v", err)
	}

	// 错误密码
	if _, err := svc.Login(dto.LoginRequest{Account: "alice", Password: "wrong"}); err == nil {
		t.Fatal("expected error for wrong password")
	}

	// 不存在的账号
	if _, err := svc.Login(dto.LoginRequest{Account: "nobody", Password: "secret123"}); err == nil {
		t.Fatal("expected error for unknown account")
	}
}

func TestAuthService_LoginDisabled(t *testing.T) {
	svc, userRepo := newAuthService(t)
	if _, err := svc.Register(dto.RegisterRequest{Username: "alice", Email: "alice@example.com", Password: "secret123"}); err != nil {
		t.Fatalf("register: %v", err)
	}

	u, _ := userRepo.GetByUsername("alice")
	if err := userRepo.UpdateFields(u.ID, map[string]any{"status": model.StatusDisabled}); err != nil {
		t.Fatalf("update user: %v", err)
	}

	if _, err := svc.Login(dto.LoginRequest{Account: "alice", Password: "secret123"}); err == nil {
		t.Fatal("expected error for disabled account")
	}
}

func TestAuthService_ChangePassword(t *testing.T) {
	svc, _ := newAuthService(t)
	user, err := svc.Register(dto.RegisterRequest{Username: "alice", Email: "alice@example.com", Password: "oldpass123"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// 旧密码错误
	if err := svc.ChangePassword(user.ID, dto.ChangePasswordRequest{OldPassword: "nope", NewPassword: "newpass123"}); err == nil {
		t.Fatal("expected error for wrong old password")
	}

	if err := svc.ChangePassword(user.ID, dto.ChangePasswordRequest{OldPassword: "oldpass123", NewPassword: "newpass123"}); err != nil {
		t.Fatalf("change password: %v", err)
	}

	// 旧密码失效
	if _, err := svc.Login(dto.LoginRequest{Account: "alice", Password: "oldpass123"}); err == nil {
		t.Fatal("expected old password to fail")
	}
	// 新密码可用
	if _, err := svc.Login(dto.LoginRequest{Account: "alice", Password: "newpass123"}); err != nil {
		t.Fatalf("login with new password: %v", err)
	}
}

func TestAuthService_UpdateProfile(t *testing.T) {
	svc, _ := newAuthService(t)
	user, _ := svc.Register(dto.RegisterRequest{Username: "alice", Email: "alice@example.com", Password: "secret123"})

	bio := "hello"
	updated, err := svc.UpdateProfile(user.ID, dto.UpdateProfileRequest{Nickname: "Alice A", Email: "new@example.com", Bio: &bio})
	if err != nil {
		t.Fatalf("update profile: %v", err)
	}
	if updated.Nickname != "Alice A" || updated.Email != "new@example.com" || updated.Bio != "hello" {
		t.Fatalf("unexpected updated profile: %+v", updated)
	}

	// 与他人邮箱冲突
	if _, err := svc.Register(dto.RegisterRequest{Username: "bob", Email: "bob@example.com", Password: "secret123"}); err != nil {
		t.Fatalf("register bob: %v", err)
	}
	if _, err := svc.UpdateProfile(user.ID, dto.UpdateProfileRequest{Email: "bob@example.com"}); err == nil {
		t.Fatal("expected duplicate email error")
	}
}

func TestAuthService_LoginLockedUnknownAccount(t *testing.T) {
	svc, _ := newAuthService(t)
	svc.SetLoginLimiter(NewLoginLimiter(3, 15))

	for range 3 {
		if _, err := svc.Login(dto.LoginRequest{Account: "nobody", Password: "x"}); err == nil {
			t.Fatal("expected error for unknown account")
		}
	}
	// 不存在的账号同样计入失败，达到阈值后应被锁定。
	if _, err := svc.Login(dto.LoginRequest{Account: "nobody", Password: "x"}); err == nil {
		t.Fatal("expected locked after repeated failures")
	}
}

func TestAuthService_RegisterAfterDelete(t *testing.T) {
	svc, userRepo := newAuthService(t)

	user, err := svc.Register(dto.RegisterRequest{Username: "alice", Email: "alice@example.com", Password: "secret123"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := userRepo.Delete(user.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// 软删除后同名/同邮箱应可重新注册。
	if _, err := svc.Register(dto.RegisterRequest{Username: "alice", Email: "alice@example.com", Password: "secret456"}); err != nil {
		t.Fatalf("re-register after delete: %v", err)
	}
}

// authTestEnv 提供更完整的测试依赖，便于覆盖新认证流程。
type authTestEnv struct {
	svc                   *AuthService
	db                    *gorm.DB
	userRepo              *repository.UserRepository
	refreshRepo           *repository.RefreshTokenRepository
	emailVerificationRepo *repository.EmailVerificationRepository
	passwordResetRepo     *repository.PasswordResetRepository
	mailer                *capturingMailer
}

func newAuthTestEnv(t *testing.T, emailVerificationEnabled bool) *authTestEnv {
	t.Helper()
	db := setupTestDB(t)
	userRepo := repository.NewUserRepository(db)
	roleRepo := repository.NewUserRoleRepository(db)
	refreshRepo := repository.NewRefreshTokenRepository(db)
	emailVerificationRepo := repository.NewEmailVerificationRepository(db)
	passwordResetRepo := repository.NewPasswordResetRepository(db)
	jwtMgr := jwt.NewManager("test-secret", 1, "test")
	tokenService := NewTokenService(refreshRepo, userRepo, roleRepo, jwtMgr, 720)
	mailer := &capturingMailer{}
	svc := NewAuthService(AuthDeps{
		UserRepo:                 userRepo,
		RoleRepo:                 roleRepo,
		TokenService:             tokenService,
		EmailVerificationRepo:    emailVerificationRepo,
		PasswordResetRepo:        passwordResetRepo,
		Mailer:                   mailer,
		EmailVerificationEnabled: emailVerificationEnabled,
	})
	return &authTestEnv{
		svc:                   svc,
		db:                    db,
		userRepo:              userRepo,
		refreshRepo:           refreshRepo,
		emailVerificationRepo: emailVerificationRepo,
		passwordResetRepo:     passwordResetRepo,
		mailer:                mailer,
	}
}

func registerAlice(t *testing.T, svc *AuthService) *dto.UserInfo {
	t.Helper()
	user, err := svc.Register(dto.RegisterRequest{Username: "alice", Email: "alice@example.com", Password: "secret123"})
	if err != nil {
		t.Fatalf("register alice: %v", err)
	}
	return user
}

func TestAuthService_EmailVerificationFlow(t *testing.T) {
	env := newAuthTestEnv(t, true)

	user := registerAlice(t, env.svc)
	if user.Status != model.StatusUnverified {
		t.Fatalf("expected unverified status, got %d", user.Status)
	}

	if _, err := env.svc.Login(dto.LoginRequest{Account: "alice", Password: "secret123"}); err == nil {
		t.Fatal("expected login to fail before verification")
	}

	var ev model.EmailVerification
	if err := env.db.Where("user_id = ?", user.ID).First(&ev).Error; err != nil {
		t.Fatalf("find verification token: %v", err)
	}
	// 库里只保存哈希，明文仅出现在邮件里。
	if ev.Token == env.mailer.verificationToken {
		t.Fatal("expected verification token to be hashed at rest")
	}
	rawToken := env.mailer.verificationToken

	if err := env.svc.VerifyEmail(dto.VerifyEmailRequest{Token: rawToken}); err != nil {
		t.Fatalf("verify email: %v", err)
	}

	resp, err := env.svc.Login(dto.LoginRequest{Account: "alice", Password: "secret123"})
	if err != nil {
		t.Fatalf("login after verify: %v", err)
	}
	if resp.Tokens.AccessToken == "" {
		t.Fatal("expected access token after verification")
	}

	if err := env.svc.VerifyEmail(dto.VerifyEmailRequest{Token: rawToken}); err == nil {
		t.Fatal("expected error reusing verification token")
	}
}

func TestAuthService_RefreshTokenRotation(t *testing.T) {
	env := newAuthTestEnv(t, false)
	registerAlice(t, env.svc)

	resp, err := env.svc.Login(dto.LoginRequest{Account: "alice", Password: "secret123"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	pair, err := env.svc.Refresh(dto.RefreshRequest{RefreshToken: resp.Tokens.RefreshToken})
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatal("expected new token pair")
	}

	// 轮换后旧 refresh 已吊销。
	if _, err := env.svc.Refresh(dto.RefreshRequest{RefreshToken: resp.Tokens.RefreshToken}); err == nil {
		t.Fatal("expected old refresh token to be revoked")
	}
}

func TestAuthService_Logout(t *testing.T) {
	env := newAuthTestEnv(t, false)
	registerAlice(t, env.svc)

	resp, err := env.svc.Login(dto.LoginRequest{Account: "alice", Password: "secret123"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	if err := env.svc.Logout(dto.LogoutRequest{RefreshToken: resp.Tokens.RefreshToken}); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, err := env.svc.Refresh(dto.RefreshRequest{RefreshToken: resp.Tokens.RefreshToken}); err == nil {
		t.Fatal("expected refresh to fail after logout")
	}
}

func TestAuthService_LogoutAll(t *testing.T) {
	env := newAuthTestEnv(t, false)
	user := registerAlice(t, env.svc)

	first, err := env.svc.Login(dto.LoginRequest{Account: "alice", Password: "secret123"})
	if err != nil {
		t.Fatalf("first login: %v", err)
	}
	second, err := env.svc.Login(dto.LoginRequest{Account: "alice", Password: "secret123"})
	if err != nil {
		t.Fatalf("second login: %v", err)
	}

	if err := env.svc.LogoutAll(user.ID); err != nil {
		t.Fatalf("logout all: %v", err)
	}

	if _, err := env.svc.Refresh(dto.RefreshRequest{RefreshToken: first.Tokens.RefreshToken}); err == nil {
		t.Fatal("expected first refresh revoked")
	}
	if _, err := env.svc.Refresh(dto.RefreshRequest{RefreshToken: second.Tokens.RefreshToken}); err == nil {
		t.Fatal("expected second refresh revoked")
	}

	// token 版本已递增。
	u, _ := env.userRepo.GetByID(user.ID)
	if u.TokenVersion == 0 {
		t.Fatal("expected token version to be incremented")
	}
}

func TestAuthService_ForgotAndResetPassword(t *testing.T) {
	env := newAuthTestEnv(t, false)
	user := registerAlice(t, env.svc)

	if err := env.svc.ForgotPassword(dto.ForgotPasswordRequest{Email: "alice@example.com"}); err != nil {
		t.Fatalf("forgot password: %v", err)
	}

	var pr model.PasswordReset
	if err := env.db.Where("user_id = ?", user.ID).First(&pr).Error; err != nil {
		t.Fatalf("find reset token: %v", err)
	}
	// 库里只保存哈希，明文仅出现在邮件里。
	if pr.Token == env.mailer.resetToken {
		t.Fatal("expected reset token to be hashed at rest")
	}

	if err := env.svc.ResetPassword(dto.ResetPasswordRequest{Token: env.mailer.resetToken, NewPassword: "newpass123"}); err != nil {
		t.Fatalf("reset password: %v", err)
	}

	// 旧密码失效。
	if _, err := env.svc.Login(dto.LoginRequest{Account: "alice", Password: "secret123"}); err == nil {
		t.Fatal("expected old password to fail")
	}
	// 新密码可用。
	if _, err := env.svc.Login(dto.LoginRequest{Account: "alice", Password: "newpass123"}); err != nil {
		t.Fatalf("login with new password: %v", err)
	}
}

func TestAuthService_DeleteAccount(t *testing.T) {
	env := newAuthTestEnv(t, false)
	user := registerAlice(t, env.svc)

	if err := env.svc.DeleteAccount(user.ID); err != nil {
		t.Fatalf("delete account: %v", err)
	}
	if _, err := env.svc.Login(dto.LoginRequest{Account: "alice", Password: "secret123"}); err == nil {
		t.Fatal("expected login to fail after account deletion")
	}
}
