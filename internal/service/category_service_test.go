package service

import (
	"testing"

	"goroutice/internal/dto"
	"goroutice/internal/model"
	"goroutice/internal/repository"
)

func newCategoryService(t *testing.T) *CategoryService {
	t.Helper()
	db := setupTestDB(t)
	return NewCategoryService(repository.NewCategoryRepository(db))
}

func TestCategoryService_Create(t *testing.T) {
	svc := newCategoryService(t)

	c, err := svc.Create(dto.CategoryRequest{Name: "Go"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if c.Slug != "go" {
		t.Fatalf("expected slug 'go', got %q", c.Slug)
	}

	// 重复名称
	if _, err := svc.Create(dto.CategoryRequest{Name: "Go"}); err == nil {
		t.Fatal("expected duplicate name error")
	}

	// 自定义 slug 已占用 -> 自动追加后缀
	c2, err := svc.Create(dto.CategoryRequest{Name: "Go Advanced", Slug: "go"})
	if err != nil {
		t.Fatalf("create with taken slug: %v", err)
	}
	if c2.Slug != "go-2" {
		t.Fatalf("expected slug 'go-2', got %q", c2.Slug)
	}
}

func TestCategoryService_UpdateAndDelete(t *testing.T) {
	svc := newCategoryService(t)
	c, _ := svc.Create(dto.CategoryRequest{Name: "Go", Description: "old"})

	updated, err := svc.Update(c.ID, dto.CategoryRequest{Name: "Golang", Description: "new", Sort: 5})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "Golang" || updated.Description != "new" || updated.Sort != 5 {
		t.Fatalf("unexpected updated: %+v", updated)
	}

	if err := svc.Delete(c.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.GetByID(c.ID); err == nil {
		t.Fatal("expected not found after delete")
	}
}

func TestCategoryService_DeleteInUse(t *testing.T) {
	db := setupTestDB(t)
	svc := NewCategoryService(repository.NewCategoryRepository(db))
	cat, _ := svc.Create(dto.CategoryRequest{Name: "Go"})

	author := createTestUser(t, db, "author", model.RoleAuthor)
	if err := repository.NewArticleRepository(db, false).Create(&model.Article{
		Title: "Post", Slug: "post", Content: "body", AuthorID: author.ID, CategoryID: cat.ID,
	}, nil); err != nil {
		t.Fatalf("create article: %v", err)
	}

	if err := svc.Delete(cat.ID); err == nil {
		t.Fatal("expected error when deleting in-use category")
	}
}

func TestCategoryService_List(t *testing.T) {
	svc := newCategoryService(t)
	if _, err := svc.Create(dto.CategoryRequest{Name: "Go"}); err != nil {
		t.Fatalf("create Go: %v", err)
	}
	if _, err := svc.Create(dto.CategoryRequest{Name: "Rust"}); err != nil {
		t.Fatalf("create Rust: %v", err)
	}

	_, total, err := svc.List(1, 10, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected 2 categories, got %d", total)
	}

	_, total, err = svc.List(1, 10, "go")
	if err != nil {
		t.Fatalf("list keyword: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected 1 category matching 'go', got %d", total)
	}
}
