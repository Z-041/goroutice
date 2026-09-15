package service

import (
	"testing"

	"goroutice/internal/dto"
	"goroutice/internal/model"
	"goroutice/internal/repository"

	"gorm.io/gorm"
)

func newArticleService(t *testing.T) (*ArticleService, *gorm.DB) {
	t.Helper()
	db := setupTestDB(t)
	svc := NewArticleService(repository.NewArticleRepository(db), repository.NewCategoryRepository(db), repository.NewTagRepository(db))
	return svc, db
}

func TestArticleService_CreateAndPublish(t *testing.T) {
	svc, db := newArticleService(t)
	author := createTestUser(t, db, "author", model.RoleAuthor)

	a, err := svc.Create(author.ID, dto.ArticleRequest{Title: "Hello World", Content: "body", Status: model.ArticleDraft})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if a.Slug != "hello-world" {
		t.Fatalf("unexpected slug %q", a.Slug)
	}

	// 草稿不在公开列表
	_, total, _ := svc.ListPublished(1, 10, "", "", "")
	if total != 0 {
		t.Fatalf("expected 0 published, got %d", total)
	}

	// 发布
	if err := svc.UpdateStatus(a.ID, model.ArticlePublished); err != nil {
		t.Fatalf("update status: %v", err)
	}

	_, total, err = svc.ListPublished(1, 10, "", "", "")
	if err != nil {
		t.Fatalf("list published: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected 1 published, got %d", total)
	}
}

func TestArticleService_GetPublished(t *testing.T) {
	svc, db := newArticleService(t)
	author := createTestUser(t, db, "author", model.RoleAuthor)

	a, _ := svc.Create(author.ID, dto.ArticleRequest{Title: "Hello", Content: "body", Status: model.ArticlePublished})

	got, err := svc.GetPublished(a.Slug)
	if err != nil {
		t.Fatalf("get published: %v", err)
	}
	if got.ViewCount != 1 {
		t.Fatalf("expected view count 1, got %d", got.ViewCount)
	}

	// 草稿不可公开访问
	draft, _ := svc.Create(author.ID, dto.ArticleRequest{Title: "Draft", Content: "body", Status: model.ArticleDraft})
	if _, err := svc.GetPublished(draft.Slug); err == nil {
		t.Fatal("expected draft not publicly accessible")
	}
}

func TestArticleService_Ownership(t *testing.T) {
	svc, db := newArticleService(t)
	authorA := createTestUser(t, db, "authorA", model.RoleAuthor)
	authorB := createTestUser(t, db, "authorB", model.RoleAuthor)

	a, _ := svc.Create(authorA.ID, dto.ArticleRequest{Title: "A's Post", Content: "body", Status: model.ArticleDraft})

	// B（非管理员）不能改/删 A 的文章
	if _, err := svc.Update(authorB.ID, []string{model.RoleAuthor}, a.ID, dto.ArticleRequest{Title: "Hacked", Content: "x"}); err == nil {
		t.Fatal("expected forbidden for non-owner update")
	}
	if err := svc.Delete(authorB.ID, []string{model.RoleAuthor}, a.ID); err == nil {
		t.Fatal("expected forbidden for non-owner delete")
	}
	if _, err := svc.GetMine(authorB.ID, []string{model.RoleAuthor}, a.ID); err == nil {
		t.Fatal("expected forbidden for non-owner view")
	}

	// A 本人可改
	if _, err := svc.Update(authorA.ID, []string{model.RoleAuthor}, a.ID, dto.ArticleRequest{Title: "Updated", Content: "x"}); err != nil {
		t.Fatalf("owner update: %v", err)
	}
	// 管理员可改/删
	if _, err := svc.Update("", []string{model.RoleAdmin}, a.ID, dto.ArticleRequest{Title: "Admin Updated", Content: "x"}); err != nil {
		t.Fatalf("admin update: %v", err)
	}
	if err := svc.Delete("", []string{model.RoleAdmin}, a.ID); err != nil {
		t.Fatalf("admin delete: %v", err)
	}
}

func TestArticleService_InvalidCategory(t *testing.T) {
	svc, db := newArticleService(t)
	author := createTestUser(t, db, "author", model.RoleAuthor)

	if _, err := svc.Create(author.ID, dto.ArticleRequest{Title: "Hello", Content: "body", CategoryID: "invalid-category-id"}); err == nil {
		t.Fatal("expected error for invalid category")
	}
}

func TestArticleService_InvalidTag(t *testing.T) {
	svc, db := newArticleService(t)
	author := createTestUser(t, db, "author", model.RoleAuthor)

	if _, err := svc.Create(author.ID, dto.ArticleRequest{Title: "Hello", Content: "body", TagIDs: []string{"invalid-tag-id"}}); err == nil {
		t.Fatal("expected error for invalid tag")
	}
}

func TestArticleService_ValidTag(t *testing.T) {
	svc, db := newArticleService(t)
	author := createTestUser(t, db, "author", model.RoleAuthor)

	tag := &model.Tag{Name: "Go", Slug: "go"}
	if err := repository.NewTagRepository(db).Create(tag); err != nil {
		t.Fatalf("create tag: %v", err)
	}

	a, err := svc.Create(author.ID, dto.ArticleRequest{Title: "Hello", Content: "body", TagIDs: []string{tag.ID}})
	if err != nil {
		t.Fatalf("create with tag: %v", err)
	}
	if len(a.Tags) != 1 || a.Tags[0].ID != tag.ID {
		t.Fatalf("expected 1 bound tag, got %+v", a.Tags)
	}
}

func TestArticleService_CreateAfterDelete(t *testing.T) {
	svc, db := newArticleService(t)
	author := createTestUser(t, db, "author", model.RoleAuthor)

	a, err := svc.Create(author.ID, dto.ArticleRequest{Title: "Hello World", Content: "body"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.Delete(author.ID, []string{model.RoleAuthor}, a.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// 软删除后同 slug 应可重新创建。
	if _, err := svc.Create(author.ID, dto.ArticleRequest{Title: "Hello World", Content: "body"}); err != nil {
		t.Fatalf("re-create same slug after delete: %v", err)
	}
}

func TestArticleService_ListMine(t *testing.T) {
	svc, db := newArticleService(t)
	authorA := createTestUser(t, db, "authorA", model.RoleAuthor)
	authorB := createTestUser(t, db, "authorB", model.RoleAuthor)

	if _, err := svc.Create(authorA.ID, dto.ArticleRequest{Title: "A1", Content: "body", Status: model.ArticleDraft}); err != nil {
		t.Fatalf("create A1: %v", err)
	}
	if _, err := svc.Create(authorA.ID, dto.ArticleRequest{Title: "A2", Content: "body", Status: model.ArticlePublished}); err != nil {
		t.Fatalf("create A2: %v", err)
	}
	if _, err := svc.Create(authorB.ID, dto.ArticleRequest{Title: "B1", Content: "body", Status: model.ArticlePublished}); err != nil {
		t.Fatalf("create B1: %v", err)
	}

	_, total, _ := svc.ListMine(authorA.ID, 1, 10, "")
	if total != 2 {
		t.Fatalf("expected 2 mine, got %d", total)
	}

	_, total, _ = svc.ListMine(authorA.ID, 1, 10, model.ArticleDraft)
	if total != 1 {
		t.Fatalf("expected 1 draft, got %d", total)
	}
}

func TestArticleService_SetFeature(t *testing.T) {
	svc, db := newArticleService(t)
	author := createTestUser(t, db, "author", model.RoleAuthor)
	a, err := svc.Create(author.ID, dto.ArticleRequest{Title: "Featured", Content: "body", Status: model.ArticlePublished})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// 空请求
	if err := svc.SetFeature(a.ID, dto.ArticleFeatureRequest{}); err == nil {
		t.Fatal("expected bad request for empty feature")
	}

	// 设置置顶 + 推荐
	pinned := true
	featured := true
	if err := svc.SetFeature(a.ID, dto.ArticleFeatureRequest{IsPinned: &pinned, IsFeatured: &featured}); err != nil {
		t.Fatalf("set feature: %v", err)
	}

	got, err := svc.GetPublished(a.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.IsPinned || !got.IsFeatured {
		t.Fatalf("expected pinned+featured, got pinned=%v featured=%v", got.IsPinned, got.IsFeatured)
	}

	// 文章不存在
	if err := svc.SetFeature("no-such-id", dto.ArticleFeatureRequest{IsPinned: &pinned}); err == nil {
		t.Fatal("expected not found for missing article")
	}
}

func TestArticleService_ListPublished_PinnedFirst(t *testing.T) {
	svc, db := newArticleService(t)
	author := createTestUser(t, db, "author", model.RoleAuthor)

	if _, err := svc.Create(author.ID, dto.ArticleRequest{Title: "A", Content: "body", Status: model.ArticlePublished}); err != nil {
		t.Fatalf("create A: %v", err)
	}
	if _, err := svc.Create(author.ID, dto.ArticleRequest{Title: "B", Content: "body", Status: model.ArticlePublished}); err != nil {
		t.Fatalf("create B: %v", err)
	}
	pinned, err := svc.Create(author.ID, dto.ArticleRequest{Title: "C", Content: "body", Status: model.ArticlePublished})
	if err != nil {
		t.Fatalf("create C: %v", err)
	}

	pt := true
	if err := svc.SetFeature(pinned.ID, dto.ArticleFeatureRequest{IsPinned: &pt}); err != nil {
		t.Fatalf("set pinned: %v", err)
	}

	items, _, err := svc.ListPublished(1, 10, "", "", "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 published, got %d", len(items))
	}
	if items[0].ID != pinned.ID {
		t.Fatalf("expected pinned article first, got %s", items[0].ID)
	}
}
