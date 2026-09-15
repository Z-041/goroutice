package service

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"goroutice/internal/dto"
	"goroutice/internal/model"
	"goroutice/internal/pkg/apperror"
	"goroutice/internal/pkg/hash"
	"goroutice/internal/pkg/token"
	"goroutice/internal/repository"

	"gorm.io/gorm"
)

// AuthDeps 汇聚 AuthService 所需依赖。
type AuthDeps struct {
	UserRepo                 *repository.UserRepository
	RoleRepo                 *repository.UserRoleRepository
	TokenService             *TokenService
	EmailVerificationRepo    *repository.EmailVerificationRepository
	PasswordResetRepo        *repository.PasswordResetRepository
	Mailer                   Mailer
	EmailVerificationEnabled bool
}

// AuthService 认证与账户服务。
type AuthService struct {
	userRepo                 *repository.UserRepository
	roleRepo                 *repository.UserRoleRepository
	tokenService             *TokenService
	emailVerificationRepo    *repository.EmailVerificationRepository
	passwordResetRepo        *repository.PasswordResetRepository
	mailer                   Mailer
	emailVerificationEnabled bool
	loginLimiter             *LoginLimiter
	passwordPolicy           PasswordPolicy
	auditor                  *AuditService
}

// NewAuthService 构造 AuthService。
func NewAuthService(deps AuthDeps) *AuthService {
	return &AuthService{
		userRepo:                 deps.UserRepo,
		roleRepo:                 deps.RoleRepo,
		tokenService:             deps.TokenService,
		emailVerificationRepo:    deps.EmailVerificationRepo,
		passwordResetRepo:        deps.PasswordResetRepo,
		mailer:                   deps.Mailer,
		emailVerificationEnabled: deps.EmailVerificationEnabled,
		passwordPolicy:           DefaultPasswordPolicy(),
	}
}

// SetLoginLimiter 注入登录失败锁定器（可选，未注入则跳过锁定逻辑）。
func (s *AuthService) SetLoginLimiter(l *LoginLimiter) {
	s.loginLimiter = l
}

// SetPasswordPolicy 覆盖默认密码强度策略。
func (s *AuthService) SetPasswordPolicy(p PasswordPolicy) {
	s.passwordPolicy = p
}

// SetAuditor 注入审计服务（可选，未注入则跳过审计）。
func (s *AuthService) SetAuditor(auditor *AuditService) {
	s.auditor = auditor
}

// Register 注册新用户；启用邮箱验证时用户初始状态为未验证，并发送验证邮件。
func (s *AuthService) Register(req dto.RegisterRequest) (*dto.UserInfo, error) {
	if err := s.checkAccountAvailable(req.Username, req.Email); err != nil {
		return nil, err
	}
	if err := s.passwordPolicy.Validate(req.Password); err != nil {
		return nil, err
	}

	hashed, err := hash.Password(req.Password)
	if err != nil {
		return nil, err
	}

	nickname := req.Nickname
	if nickname == "" {
		nickname = req.Username
	}

	status := model.StatusActive
	if s.emailVerificationEnabled {
		status = model.StatusUnverified
	}

	user := &model.User{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: hashed,
		Nickname:     nickname,
		Role:         model.RoleUser,
		Status:       status,
	}
	if err := s.userRepo.CreateWithRoles(user, []string{model.RoleUser}); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, apperror.Conflict("username or email already exists")
		}
		return nil, err
	}

	if s.emailVerificationEnabled {
		// 账号已创建成功，发信失败只影响首次可达性（可通过重发接口补救），
		// 不应把注册整体判为失败而让调用方以为需要重新注册。
		if err := s.sendVerification(user); err != nil {
			slog.Error("send verification email", "err", err, "user_id", user.ID)
		}
	}
	s.auditor.Record(Operator{ID: user.ID, Username: user.Username}, model.AuditRegister, "role="+model.RoleUser)
	return dto.ToUserInfoWithRoles(user, []string{model.RoleUser}), nil
}

// Login 用户登录，Account 支持用户名或邮箱。
func (s *AuthService) Login(req dto.LoginRequest) (*dto.LoginResponse, error) {
	if s.loginLimiter != nil && s.loginLimiter.Locked(req.Account) {
		return nil, apperror.TooManyRequests("too many failed attempts, please try again later")
	}

	user, err := s.findUserByAccount(req.Account)
	if err != nil {
		return nil, err
	}

	if !hash.CheckPassword(user.PasswordHash, req.Password) {
		if s.loginLimiter != nil {
			s.loginLimiter.Fail(req.Account)
		}
		s.auditor.Record(Operator{ID: user.ID, Username: user.Username}, model.AuditLoginFailed, "invalid credentials")
		return nil, apperror.Unauthorized("invalid account or password")
	}

	if err := s.checkLoginStatus(user); err != nil {
		return nil, err
	}

	if s.loginLimiter != nil {
		s.loginLimiter.Success(req.Account)
	}

	roles, err := s.roleRepo.GetRolesByUserID(user.ID)
	if err != nil {
		return nil, err
	}

	pair, err := s.tokenService.IssueTokens(user.ID, user.Username, roles, user.TokenVersion)
	if err != nil {
		return nil, err
	}
	s.auditor.Record(Operator{ID: user.ID, Username: user.Username}, model.AuditLogin, "account="+req.Account)

	return &dto.LoginResponse{
		Tokens: *pair,
		User:   dto.ToUserInfoWithRoles(user, roles),
	}, nil
}

// findUserByAccount 按用户名或邮箱查找用户；未命中计入失败并返回统一错误。
func (s *AuthService) findUserByAccount(account string) (*model.User, error) {
	user, err := s.userRepo.GetByUsername(account)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		user, err = s.userRepo.GetByEmail(account)
	}
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if s.loginLimiter != nil {
				s.loginLimiter.Fail(account)
			}
			s.auditor.Record(Operator{Username: account}, model.AuditLoginFailed, "account not found")
			return nil, apperror.Unauthorized("invalid account or password")
		}
		return nil, err
	}
	return user, nil
}

// checkLoginStatus 校验用户状态是否允许登录。
func (s *AuthService) checkLoginStatus(user *model.User) error {
	switch user.Status {
	case model.StatusDisabled:
		return apperror.Forbidden("account is disabled")
	case model.StatusUnverified:
		return apperror.Forbidden("email not verified")
	}
	return nil
}

// Refresh 轮换 refresh token。
func (s *AuthService) Refresh(req dto.RefreshRequest) (*dto.TokenPair, error) {
	return s.tokenService.Refresh(req.RefreshToken)
}

// Logout 登出，吊销单个 refresh token。
func (s *AuthService) Logout(req dto.LogoutRequest) error {
	return s.tokenService.Revoke(req.RefreshToken)
}

// tokenVersionBump 返回使该用户已签发的 access token 全部失效的字段更新。
// 版本号交给数据库自增，避免读-改-写并发下只加一次。
func tokenVersionBump() map[string]any {
	return map[string]any{"token_version": gorm.Expr("token_version + 1")}
}

// LogoutAll 全端下线：递增 token 版本并使所有 refresh token 失效。
func (s *AuthService) LogoutAll(userID string) error {
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return err
	}
	if err := s.userRepo.UpdateFields(userID, tokenVersionBump()); err != nil {
		return err
	}
	if err := s.tokenService.RevokeAll(userID); err != nil {
		return err
	}
	s.auditor.Record(Operator{ID: user.ID, Username: user.Username}, model.AuditLogoutAll, "all sessions revoked")
	return nil
}

// DeleteAccount 注销当前账号：软删用户、清理角色与令牌。
func (s *AuthService) DeleteAccount(userID string) error {
	username := ""
	if user, err := s.userRepo.GetByID(userID); err == nil {
		username = user.Username
	}

	if err := s.userRepo.Delete(userID); err != nil {
		return err
	}
	// 账号已删除，后续清理失败只记日志：返回错误会让调用方误以为注销未生效。
	if err := s.roleRepo.ReplaceRoles(userID, nil); err != nil {
		slog.Error("cleanup roles after account deletion", "err", err, "user_id", userID)
	}
	if err := s.tokenService.RevokeAll(userID); err != nil {
		slog.Error("cleanup refresh tokens after account deletion", "err", err, "user_id", userID)
	}
	if err := s.emailVerificationRepo.DeleteByUserID(userID); err != nil {
		slog.Error("cleanup email verifications after account deletion", "err", err, "user_id", userID)
	}
	if err := s.passwordResetRepo.DeleteByUserID(userID); err != nil {
		slog.Error("cleanup password resets after account deletion", "err", err, "user_id", userID)
	}
	s.auditor.Record(Operator{ID: userID, Username: username}, model.AuditDeleteAccount, "account deleted")
	return nil
}

// VerifyEmail 校验邮箱验证令牌并激活账号。
func (s *AuthService) VerifyEmail(req dto.VerifyEmailRequest) error {
	// 库里存的是哈希，查询前先对来参做同样处理。
	ev, err := s.emailVerificationRepo.GetByToken(token.Hash(req.Token))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperror.BadRequest("invalid token")
		}
		return err
	}
	if ev.UsedAt != nil {
		return apperror.BadRequest("token already used")
	}
	if time.Now().After(ev.ExpiresAt) {
		return apperror.BadRequest("token expired")
	}

	user, err := s.userRepo.GetByID(ev.UserID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperror.NotFound("user not found")
		}
		return err
	}
	if user.Status != model.StatusUnverified {
		return apperror.BadRequest("email already verified")
	}

	// 先原子消费令牌再激活，避免同一令牌被并发重复使用。
	consumed, err := s.emailVerificationRepo.MarkUsed(ev.ID, time.Now())
	if err != nil {
		return err
	}
	if !consumed {
		return apperror.BadRequest("token already used")
	}
	if err := s.userRepo.UpdateFields(user.ID, map[string]any{
		"status": model.StatusActive,
	}); err != nil {
		return err
	}
	s.auditor.Record(Operator{ID: user.ID, Username: user.Username}, model.AuditVerifyEmail, "email verified")
	return nil
}

// ResendVerification 重新发送邮箱验证邮件。
// 邮箱未注册时同样静默成功，与 ForgotPassword 保持一致的防枚举语义。
func (s *AuthService) ResendVerification(req dto.ResendVerificationRequest) error {
	user, err := s.userRepo.GetByEmail(req.Email)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if user.Status != model.StatusUnverified {
		return apperror.BadRequest("email already verified")
	}
	return s.sendVerification(user)
}

// ForgotPassword 发送密码重置邮件；为避免泄露账号是否存在，未命中时静默成功。
func (s *AuthService) ForgotPassword(req dto.ForgotPasswordRequest) error {
	user, err := s.userRepo.GetByEmail(req.Email)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}

	raw, err := token.Generate()
	if err != nil {
		return err
	}
	pr := &model.PasswordReset{
		UserID: user.ID,
		// 只存哈希，理由同邮箱验证令牌。
		Token:     token.Hash(raw),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := s.passwordResetRepo.Create(pr); err != nil {
		return err
	}
	// 令牌已生成，发信失败只影响本次可达性（可重试）。此处若返回错误，
	// 「邮箱已注册」会变成 500 而「未注册」仍是 200，反而暴露账号是否存在。
	if err := s.mailer.SendPasswordResetEmail(user.Email, raw); err != nil {
		slog.Error("send password reset email", "err", err, "user_id", user.ID)
	}
	return nil
}

// ResetPassword 校验重置令牌并更新密码，同时使旧登录态失效。
func (s *AuthService) ResetPassword(req dto.ResetPasswordRequest) error {
	// 库里存的是哈希，查询前先对来参做同样处理。
	pr, err := s.passwordResetRepo.GetByToken(token.Hash(req.Token))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperror.BadRequest("invalid token")
		}
		return err
	}
	if pr.UsedAt != nil {
		return apperror.BadRequest("token already used")
	}
	if time.Now().After(pr.ExpiresAt) {
		return apperror.BadRequest("token expired")
	}
	if err := s.passwordPolicy.Validate(req.NewPassword); err != nil {
		return err
	}

	user, err := s.userRepo.GetByID(pr.UserID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperror.NotFound("user not found")
		}
		return err
	}

	hashed, err := hash.Password(req.NewPassword)
	if err != nil {
		return err
	}
	// 先原子消费令牌：并发携带同一令牌时只有一个请求能完成重置。
	consumed, err := s.passwordResetRepo.MarkUsed(pr.ID, time.Now())
	if err != nil {
		return err
	}
	if !consumed {
		return apperror.BadRequest("token already used")
	}
	// 只写密码与版本号，避免把内存快照里的其它字段一并回写。
	// 写入与吊销全部会话放在同一事务，杜绝「密码已改但旧会话仍有效」的中间态。
	if err := s.userRepo.UpdatePasswordAndRevokeSessions(user.ID, hashed); err != nil {
		return err
	}
	s.auditor.Record(Operator{ID: user.ID, Username: user.Username}, model.AuditResetPassword, "password reset via email token")
	return nil
}

// GetProfile 获取当前用户资料。
func (s *AuthService) GetProfile(userID string) (*dto.UserInfo, error) {
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.NotFound("user not found")
		}
		return nil, err
	}
	roles, err := s.roleRepo.GetRolesByUserID(userID)
	if err != nil {
		return nil, err
	}
	return dto.ToUserInfoWithRoles(user, roles), nil
}

// UpdateProfile 更新当前用户资料。
func (s *AuthService) UpdateProfile(userID string, req dto.UpdateProfileRequest) (*dto.UserInfo, error) {
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.NotFound("user not found")
		}
		return nil, err
	}

	// 只写入请求里显式提供的字段：无条件回写 avatar/bio 会把未提交的资料清空。
	fields := map[string]any{}
	if req.Avatar != nil {
		fields["avatar"] = *req.Avatar
		user.Avatar = *req.Avatar
	}
	if req.Bio != nil {
		fields["bio"] = *req.Bio
		user.Bio = *req.Bio
	}

	if req.Email != "" && req.Email != user.Email {
		if _, err := s.userRepo.GetByEmail(req.Email); err == nil {
			return nil, apperror.Conflict("email already exists")
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		fields["email"] = req.Email
		user.Email = req.Email
	}
	if req.Nickname != "" {
		fields["nickname"] = req.Nickname
		user.Nickname = req.Nickname
	}

	if err := s.userRepo.UpdateFields(userID, fields); err != nil {
		// 并发改同一邮箱时唯一索引兜底：把冲突翻译成 409 而不是 500。
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, apperror.Conflict("email already exists")
		}
		return nil, err
	}
	roles, err := s.roleRepo.GetRolesByUserID(userID)
	if err != nil {
		return nil, err
	}
	return dto.ToUserInfoWithRoles(user, roles), nil
}

// ChangePassword 修改当前用户密码，并使旧登录态失效。
func (s *AuthService) ChangePassword(userID string, req dto.ChangePasswordRequest) error {
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperror.NotFound("user not found")
		}
		return err
	}

	if !hash.CheckPassword(user.PasswordHash, req.OldPassword) {
		return apperror.BadRequest("old password is incorrect")
	}
	if err := s.passwordPolicy.Validate(req.NewPassword); err != nil {
		return err
	}

	hashed, err := hash.Password(req.NewPassword)
	if err != nil {
		return err
	}
	// 只写密码与版本号，避免把内存快照里的其它字段一并回写。
	// 写入与吊销全部会话放在同一事务，杜绝「密码已改但旧会话仍有效」的中间态。
	if err := s.userRepo.UpdatePasswordAndRevokeSessions(userID, hashed); err != nil {
		return err
	}
	s.auditor.Record(Operator{ID: user.ID, Username: user.Username}, model.AuditChangePassword, "password changed")
	return nil
}

// checkAccountAvailable 校验用户名与邮箱未被占用。
func (s *AuthService) checkAccountAvailable(username, email string) error {
	if _, err := s.userRepo.GetByUsername(username); err == nil {
		return apperror.Conflict("username already exists")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if _, err := s.userRepo.GetByEmail(email); err == nil {
		return apperror.Conflict("email already exists")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return nil
}

// sendVerification 生成邮箱验证令牌并发送。
func (s *AuthService) sendVerification(user *model.User) error {
	raw, err := token.Generate()
	if err != nil {
		return err
	}
	ev := &model.EmailVerification{
		UserID: user.ID,
		// 只存哈希：库/备份泄露时不能直接拿去激活或重置账号（邮件里仍是明文令牌）。
		Token:     token.Hash(raw),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	if err := s.emailVerificationRepo.Create(ev); err != nil {
		return err
	}
	return s.mailer.SendVerificationEmail(user.Email, raw)
}

// PasswordPolicy 密码强度策略。
type PasswordPolicy struct {
	MinLength        int
	RequireUppercase bool
	RequireSpecial   bool
}

// maxPasswordBytes 是 bcrypt 能处理的密码字节上限，超出会直接返回错误。
const maxPasswordBytes = 72

// DefaultPasswordPolicy 返回默认策略：至少 8 位，且同时包含字母与数字。
func DefaultPasswordPolicy() PasswordPolicy {
	return PasswordPolicy{MinLength: 8}
}

// Validate 校验密码是否满足强度策略。
func (p PasswordPolicy) Validate(password string) error {
	min := p.MinLength
	if min <= 0 {
		min = 8
	}
	if len(password) < min {
		return apperror.BadRequest(fmt.Sprintf("password must be at least %d characters", min))
	}
	// 密码长度按 DTO 最多 64 个字符，但多字节字符会超过 bcrypt 的 72 字节上限，
	// 提前拦成 400，避免注册/改密以 500 结束。
	if len(password) > maxPasswordBytes {
		return apperror.BadRequest(fmt.Sprintf("password must be at most %d bytes", maxPasswordBytes))
	}

	classes := classifyPassword(password)
	if !classes.hasLetter || !classes.hasDigit {
		return apperror.BadRequest("password must contain both letters and digits")
	}
	if p.RequireUppercase && !classes.hasUpper {
		return apperror.BadRequest("password must contain an uppercase letter")
	}
	if p.RequireSpecial && !classes.hasSpecial {
		return apperror.BadRequest("password must contain a special character")
	}
	return nil
}

// passwordClasses 记录密码中包含的字符类别。
type passwordClasses struct {
	hasLetter  bool
	hasDigit   bool
	hasUpper   bool
	hasSpecial bool
}

// classifyPassword 统计密码中的字符类别。
func classifyPassword(password string) passwordClasses {
	var c passwordClasses
	for _, r := range password {
		switch {
		case r >= 'a' && r <= 'z':
			c.hasLetter = true
		case r >= 'A' && r <= 'Z':
			c.hasLetter = true
			c.hasUpper = true
		case r >= '0' && r <= '9':
			c.hasDigit = true
		default:
			c.hasSpecial = true
		}
	}
	return c
}

// validatePassword 使用默认策略校验密码强度（供测试与兼容调用）。
func validatePassword(p string) error {
	return DefaultPasswordPolicy().Validate(p)
}
