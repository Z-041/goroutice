package service

import (
	"testing"

	"goroutice/internal/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// setupTestDB 创建内存 SQLite 并自动迁移全部模型。
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	// 内存数据库必须保持单连接，否则各连接间数据不可见。
	sqlDB.SetMaxOpenConns(1)

	if err := db.AutoMigrate(&model.User{}, &model.UserRole{}, &model.Category{}, &model.Tag{}, &model.Article{}, &model.ArticleRevision{}, &model.File{}, &model.PermissionAudit{}, &model.RefreshToken{}, &model.EmailVerification{}, &model.PasswordReset{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

// capturingMailer 记录最近一次下发的验证/重置令牌。
// 令牌在库里只存哈希，测试只能从邮件这一侧取出明文。
type capturingMailer struct {
	verificationToken string
	resetToken        string
}

// SendVerificationEmail 记录验证令牌。
func (m *capturingMailer) SendVerificationEmail(_, token string) error {
	m.verificationToken = token
	return nil
}

// SendPasswordResetEmail 记录重置令牌。
func (m *capturingMailer) SendPasswordResetEmail(_, token string) error {
	m.resetToken = token
	return nil
}

// createTestUser 直接创建测试用户（含角色关联）并返回模型。
func createTestUser(t *testing.T, db *gorm.DB, username, role string) *model.User {
	t.Helper()

	u := &model.User{
		Username:     username,
		Email:        username + "@test.com",
		PasswordHash: "hashed",
		Role:         role,
		Status:       model.StatusActive,
	}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("create user %q: %v", username, err)
	}
	if err := db.Create(&model.UserRole{UserID: u.ID, Role: role}).Error; err != nil {
		t.Fatalf("create role for %q: %v", username, err)
	}
	return u
}
