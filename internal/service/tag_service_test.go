package service

import (
	"testing"

	"goroutice/internal/dto"
	"goroutice/internal/repository"
)

func TestTagService_CreateUpdateDelete(t *testing.T) {
	svc := NewTagService(repository.NewTagRepository(setupTestDB(t)))

	tag, err := svc.Create(dto.TagRequest{Name: "Go"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if tag.Slug != "go" {
		t.Fatalf("expected slug 'go', got %q", tag.Slug)
	}

	// 重复名称
	if _, err := svc.Create(dto.TagRequest{Name: "Go"}); err == nil {
		t.Fatal("expected duplicate name error")
	}

	updated, err := svc.Update(tag.ID, dto.TagRequest{Name: "Golang"})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "Golang" {
		t.Fatalf("expected name 'Golang', got %q", updated.Name)
	}

	if err := svc.Delete(tag.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.GetByID(tag.ID); err == nil {
		t.Fatal("expected not found after delete")
	}
}
