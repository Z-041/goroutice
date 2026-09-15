package repository

import (
	"testing"

	"goroutice/internal/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupRepoTestDB(t *testing.T) *gorm.DB {
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

	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func createRepoUser(t *testing.T, db *gorm.DB, username, email, nickname string) *model.User {
	t.Helper()

	u := &model.User{
		Username:     username,
		Email:        email,
		Nickname:     nickname,
		PasswordHash: "hashed",
		Role:         model.RoleUser,
		Status:       model.StatusActive,
	}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return u
}

func TestUserRepositoryListFilterAndPage(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewUserRepository(db)

	createRepoUser(t, db, "alice", "alice@test.com", "Alice A")
	createRepoUser(t, db, "bob", "bob@test.com", "Bob B")
	createRepoUser(t, db, "carol", "carol@test.com", "Alice C")

	// 关键字过滤昵称
	_, total, err := repo.List(1, 10, "Alice")
	if err != nil {
		t.Fatalf("list filtered: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected 2 matches, got %d", total)
	}

	// 分页
	_, total, err = repo.List(1, 1, "")
	if err != nil {
		t.Fatalf("list page 1: %v", err)
	}
	if total != 3 {
		t.Fatalf("expected total 3, got %d", total)
	}
}

func TestUserRepositorySoftDelete(t *testing.T) {
	db := setupRepoTestDB(t)
	repo := NewUserRepository(db)

	u := createRepoUser(t, db, "alice", "alice@test.com", "Alice")

	if err := repo.Delete(u.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if _, err := repo.GetByID(u.ID); err == nil {
		t.Fatal("expected not found after soft delete")
	}

	// 软删除后同名/同邮箱可重新创建。
	again := &model.User{
		Username:     "alice",
		Email:        "alice@test.com",
		PasswordHash: "hashed",
		Role:         model.RoleUser,
		Status:       model.StatusActive,
	}
	if err := db.Create(again).Error; err != nil {
		t.Fatalf("recreate after delete: %v", err)
	}
}
