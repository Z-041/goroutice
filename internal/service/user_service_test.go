package service

import (
	"testing"

	"goroutice/internal/dto"
	"goroutice/internal/model"
	"goroutice/internal/repository"
)

func TestUserService_ListUpdateDelete(t *testing.T) {
	db := setupTestDB(t)
	svc := NewUserService(repository.NewUserRepository(db), repository.NewUserRoleRepository(db))

	alice := createTestUser(t, db, "alice", model.RoleUser)
	createTestUser(t, db, "bob", model.RoleUser)

	_, total, err := svc.List(1, 10, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected 2 users, got %d", total)
	}

	// 更新状态
	status := model.StatusDisabled
	updated, err := svc.Update(alice.ID, dto.UpdateUserRequest{Status: &status})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Status != model.StatusDisabled {
		t.Fatalf("unexpected updated: %+v", updated)
	}

	// 非法状态
	badStatus := 5
	if _, err := svc.Update(alice.ID, dto.UpdateUserRequest{Status: &badStatus}); err == nil {
		t.Fatal("expected error for invalid status")
	}

	// 删除
	if err := svc.Delete(alice.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, total, _ = svc.List(1, 10, "")
	if total != 1 {
		t.Fatalf("expected 1 user after delete, got %d", total)
	}
}
