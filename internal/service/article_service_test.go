package service

import (
	"errors"
	"net/http"
	"strconv"
	"testing"

	"goroutice/internal/dto"
	"goroutice/internal/model"
	"goroutice/internal/pkg/apperror"
	"goroutice/internal/repository"

	"gorm.io/gorm"
)

// testRevisionKeep 是测试中每篇文章保留的修订条数。取小值以便直接验证裁剪边界。
const testRevisionKeep = 3

// testViewDedupMinutes 是测试中浏览量去重的窗口（分钟）。
// 用默认窗口即可：需要验证窗口过期时由 view_tracker_test.go 直接控制时钟。
const testViewDedupMinutes = 30

func newArticleService(t *testing.T) (*ArticleService, *gorm.DB) {
	t.Helper()
	db := setupTestDB(t)
	// 第二个参数为 false：SQLite 没有 FULLTEXT，检索走 LIKE 分支。
	svc := NewArticleService(
		repository.NewArticleRepository(db, false),
		repository.NewCategoryRepository(db),
		repository.NewTagRepository(db),
		repository.NewArticleRevisionRepository(db),
		testRevisionKeep,
		NewViewTracker(testViewDedupMinutes),
	)
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

	got, err := svc.GetPublished(a.Slug, "10.0.0.1|test-agent")
	if err != nil {
		t.Fatalf("get published: %v", err)
	}
	if got.ViewCount != 1 {
		t.Fatalf("expected view count 1, got %d", got.ViewCount)
	}

	// 草稿不可公开访问
	draft, _ := svc.Create(author.ID, dto.ArticleRequest{Title: "Draft", Content: "body", Status: model.ArticleDraft})
	if _, err := svc.GetPublished(draft.Slug, "10.0.0.1|test-agent"); err == nil {
		t.Fatal("expected draft not publicly accessible")
	}
}

// TestArticleService_ViewDedup 覆盖浏览量去重：同一来源窗口内重复打开只计一次，
// 换来源（另一个 IP 或另一个浏览器）才计新的一次。
func TestArticleService_ViewDedup(t *testing.T) {
	svc, db := newArticleService(t)
	author := createTestUser(t, db, "author", model.RoleAuthor)
	a, _ := svc.Create(author.ID, dto.ArticleRequest{Title: "Hello", Content: "body", Status: model.ArticlePublished})

	for i := range 3 {
		got, err := svc.GetPublished(a.Slug, "10.0.0.1|agent-a")
		if err != nil {
			t.Fatalf("get published #%d: %v", i+1, err)
		}
		if got.ViewCount != 1 {
			t.Fatalf("第 %d 次访问后 view_count = %d，期望仍为 1（同一来源应被去重）", i+1, got.ViewCount)
		}
	}

	// 同一 IP 换浏览器：UA 不同即视为另一个访客。
	if got, _ := svc.GetPublished(a.Slug, "10.0.0.1|agent-b"); got.ViewCount != 2 {
		t.Fatalf("view_count = %d，期望 2（UA 不同应计新的一次）", got.ViewCount)
	}
	// 不同 IP 的同款浏览器。
	if got, _ := svc.GetPublished(a.Slug, "10.0.0.2|agent-a"); got.ViewCount != 3 {
		t.Fatalf("view_count = %d，期望 3（IP 不同应计新的一次）", got.ViewCount)
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

	_, total, _ := svc.ListMine(authorA.ID, 1, 10, "", "", "", "")
	if total != 2 {
		t.Fatalf("expected 2 mine, got %d", total)
	}

	_, total, _ = svc.ListMine(authorA.ID, 1, 10, model.ArticleDraft, "", "", "")
	if total != 1 {
		t.Fatalf("expected 1 draft, got %d", total)
	}

	// 关键词过滤必须对作者自己的列表同样生效：前端「我的文章」上的搜索框与
	// 管理端共用同一套查询参数，只支持 status 的话搜索会静默地什么也不做。
	_, total, _ = svc.ListMine(authorA.ID, 1, 10, "", "A2", "", "")
	if total != 1 {
		t.Fatalf("expected 1 matched by keyword, got %d", total)
	}
	_, total, _ = svc.ListMine(authorA.ID, 1, 10, "", "B1", "", "")
	if total != 0 {
		t.Fatalf("keyword must not match other authors' articles, got %d", total)
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

	got, err := svc.GetPublished(a.ID, "10.0.0.1|test-agent")
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

func TestArticleService_RevisionHistoryAndRestore(t *testing.T) {
	svc, db := newArticleService(t)
	author := createTestUser(t, db, "author", model.RoleAuthor)
	roles := []string{model.RoleAuthor}

	a, err := svc.Create(author.ID, dto.ArticleRequest{
		Title: "第一版", Slug: "first", Summary: "摘要一", Content: "正文一", Status: model.ArticlePublished,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// 创建阶段不产生修订：此时「当前内容」就是第一版本身，没有必要留一份完全相同的副本。
	if _, total, _ := svc.ListRevisions(author.ID, roles, a.ID, 1, 10); total != 0 {
		t.Fatalf("expected no revision after create, got %d", total)
	}

	if _, err := svc.Update(author.ID, roles, a.ID, dto.ArticleRequest{
		Title: "第二版", Slug: "second", Summary: "摘要二", Content: "正文二", Status: model.ArticlePublished,
	}); err != nil {
		t.Fatalf("update: %v", err)
	}

	revisions, total, err := svc.ListRevisions(author.ID, roles, a.ID, 1, 10)
	if err != nil {
		t.Fatalf("list revisions: %v", err)
	}
	if total != 1 || len(revisions) != 1 {
		t.Fatalf("expected 1 revision, got %d", total)
	}
	if revisions[0].Version != 1 {
		t.Fatalf("expected version 1, got %d", revisions[0].Version)
	}
	// 修订记录的是「更新前」的状态，因此第一条修订就是第一版。
	if revisions[0].Title != "第一版" || revisions[0].Slug != "first" {
		t.Fatalf("revision 内容不是更新前的状态: %+v", revisions[0])
	}

	detail, err := svc.GetRevision(author.ID, roles, a.ID, revisions[0].ID)
	if err != nil {
		t.Fatalf("get revision: %v", err)
	}
	if detail.Content != "正文一" || detail.Summary != "摘要一" {
		t.Fatalf("修订详情不完整: %+v", detail)
	}

	restored, err := svc.RestoreRevision(author.ID, roles, a.ID, revisions[0].ID)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if restored.Title != "第一版" || restored.Content != "正文一" || restored.Slug != "first" {
		t.Fatalf("回滚后的内容不正确: %+v", restored)
	}

	// 回滚前会先把当前内容存成修订，因此回滚本身也可以被再回滚。
	if _, total, _ := svc.ListRevisions(author.ID, roles, a.ID, 1, 10); total != 2 {
		t.Fatalf("expected restore to append a revision, got %d", total)
	}
}

func TestArticleService_RevisionKeepsTags(t *testing.T) {
	svc, db := newArticleService(t)
	author := createTestUser(t, db, "author", model.RoleAuthor)
	roles := []string{model.RoleAuthor}
	tagRepo := repository.NewTagRepository(db)

	tagA := &model.Tag{Name: "Go", Slug: "go"}
	tagB := &model.Tag{Name: "Rust", Slug: "rust"}
	for _, tag := range []*model.Tag{tagA, tagB} {
		if err := tagRepo.Create(tag); err != nil {
			t.Fatalf("create tag: %v", err)
		}
	}

	a, err := svc.Create(author.ID, dto.ArticleRequest{Title: "Post", Content: "body", TagIDs: []string{tagA.ID}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Update(author.ID, roles, a.ID, dto.ArticleRequest{Title: "Post", Content: "body", TagIDs: []string{tagB.ID}}); err != nil {
		t.Fatalf("update: %v", err)
	}

	revisions, _, err := svc.ListRevisions(author.ID, roles, a.ID, 1, 10)
	if err != nil {
		t.Fatalf("list revisions: %v", err)
	}
	detail, err := svc.GetRevision(author.ID, roles, a.ID, revisions[0].ID)
	if err != nil {
		t.Fatalf("get revision: %v", err)
	}
	if len(detail.TagIDs) != 1 || detail.TagIDs[0] != tagA.ID {
		t.Fatalf("修订未保存标签快照: %+v", detail.TagIDs)
	}

	restored, err := svc.RestoreRevision(author.ID, roles, a.ID, revisions[0].ID)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if len(restored.Tags) != 1 || restored.Tags[0].ID != tagA.ID {
		t.Fatalf("回滚未恢复标签: %+v", restored.Tags)
	}
}

func TestArticleService_RevisionPrunedToKeep(t *testing.T) {
	svc, db := newArticleService(t)
	author := createTestUser(t, db, "author", model.RoleAuthor)
	roles := []string{model.RoleAuthor}

	a, err := svc.Create(author.ID, dto.ArticleRequest{Title: "Post", Content: "v0"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// 更新次数超过保留上限，历史应被裁剪到 keep 条而不是无限增长。
	for i := range testRevisionKeep + 2 {
		if _, err := svc.Update(author.ID, roles, a.ID, dto.ArticleRequest{
			Title: "Post", Content: "v" + strconv.Itoa(i),
		}); err != nil {
			t.Fatalf("update %d: %v", i, err)
		}
	}

	revisions, total, err := svc.ListRevisions(author.ID, roles, a.ID, 1, 100)
	if err != nil {
		t.Fatalf("list revisions: %v", err)
	}
	if total != testRevisionKeep {
		t.Fatalf("expected %d revisions kept, got %d", testRevisionKeep, total)
	}
	// 裁剪的是最旧的版本，最新一条必须保留下来。
	if revisions[0].Version != testRevisionKeep+2 {
		t.Fatalf("expected newest revision version %d, got %d", testRevisionKeep+2, revisions[0].Version)
	}
}

func TestArticleService_RevisionAccessControl(t *testing.T) {
	svc, db := newArticleService(t)
	authorA := createTestUser(t, db, "authorA", model.RoleAuthor)
	authorB := createTestUser(t, db, "authorB", model.RoleAuthor)

	a, _ := svc.Create(authorA.ID, dto.ArticleRequest{Title: "A's Post", Content: "v1"})
	if _, err := svc.Update(authorA.ID, []string{model.RoleAuthor}, a.ID, dto.ArticleRequest{Title: "A's Post", Content: "v2"}); err != nil {
		t.Fatalf("update: %v", err)
	}
	revisions, _, err := svc.ListRevisions(authorA.ID, []string{model.RoleAuthor}, a.ID, 1, 10)
	if err != nil {
		t.Fatalf("list revisions: %v", err)
	}

	// 非作者既不能看历史也不能回滚
	if _, _, err := svc.ListRevisions(authorB.ID, []string{model.RoleAuthor}, a.ID, 1, 10); err == nil {
		t.Fatal("expected forbidden when listing others' revisions")
	}
	if _, err := svc.GetRevision(authorB.ID, []string{model.RoleAuthor}, a.ID, revisions[0].ID); err == nil {
		t.Fatal("expected forbidden when reading others' revision")
	}
	if _, err := svc.RestoreRevision(authorB.ID, []string{model.RoleAuthor}, a.ID, revisions[0].ID); err == nil {
		t.Fatal("expected forbidden when restoring others' revision")
	}

	// 修订必须属于目标文章：否则拿别的文章的修订 ID 就能把内容灌进来。
	other, _ := svc.Create(authorB.ID, dto.ArticleRequest{Title: "B's Post", Content: "b1"})
	if _, err := svc.RestoreRevision(authorB.ID, []string{model.RoleAuthor}, other.ID, revisions[0].ID); err == nil {
		t.Fatal("expected not found for revision of another article")
	}
}

// assertConflict 断言错误是 409 版本冲突。
func assertConflict(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("期望 409 版本冲突，实际写入成功了")
	}
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Status != http.StatusConflict {
		t.Fatalf("期望 409 版本冲突，实际为 %v", err)
	}
}

// TestArticleService_VersionConflict 覆盖乐观锁：带着过期 version 的提交必须被拒绝，
// 而不是把别人刚写入的内容静默覆盖掉。
func TestArticleService_VersionConflict(t *testing.T) {
	svc, db := newArticleService(t)
	author := createTestUser(t, db, "author", model.RoleAuthor)
	roles := []string{model.RoleAuthor}

	a, err := svc.Create(author.ID, dto.ArticleRequest{Title: "Post", Content: "v1"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if a.Version != 1 {
		t.Fatalf("新建文章 version = %d，期望 1", a.Version)
	}

	// 带正确版本提交：写入成功且版本自增。
	updated, err := svc.Update(author.ID, roles, a.ID, dto.ArticleRequest{
		Title: "Post", Content: "v2", Version: a.Version,
	})
	if err != nil {
		t.Fatalf("带最新 version 的更新失败: %v", err)
	}
	if updated.Version != a.Version+1 {
		t.Fatalf("更新后 version = %d，期望 %d", updated.Version, a.Version+1)
	}

	// 复用已被推进过的旧版本号：必须 409，且正文保持他人写入的结果。
	if _, err := svc.Update(author.ID, roles, a.ID, dto.ArticleRequest{
		Title: "Post", Content: "stale overwrite", Version: a.Version,
	}); err != nil {
		assertConflict(t, err)
	} else {
		t.Fatal("旧 version 的提交被接受了，内容会被静默覆盖")
	}

	got, err := svc.GetMine(author.ID, roles, a.ID)
	if err != nil {
		t.Fatalf("get mine: %v", err)
	}
	if got.Content != "v2" {
		t.Fatalf("冲突提交不应落库，正文 = %q，期望仍是 v2", got.Content)
	}

	// version 传 0（省略）表示不做并发检查：尚未接入该字段的客户端仍能正常保存。
	if _, err := svc.Update(author.ID, roles, a.ID, dto.ArticleRequest{Title: "Post", Content: "v3"}); err != nil {
		t.Fatalf("不带 version 的更新应保持向后兼容: %v", err)
	}
}

// TestArticleService_VersionConflict_StatusBump 覆盖「状态变更也推进版本」：
// 管理员归档后，作者拿编辑页里的旧版本提交，不应把状态悄悄改回已发布。
func TestArticleService_VersionConflict_StatusBump(t *testing.T) {
	svc, db := newArticleService(t)
	author := createTestUser(t, db, "author", model.RoleAuthor)
	roles := []string{model.RoleAuthor}

	a, err := svc.Create(author.ID, dto.ArticleRequest{
		Title: "Post", Content: "v1", Status: model.ArticlePublished,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	stale := a.Version

	if err := svc.UpdateStatus(a.ID, model.ArticleArchived); err != nil {
		t.Fatalf("update status: %v", err)
	}

	if _, err := svc.Update(author.ID, roles, a.ID, dto.ArticleRequest{
		Title: "Post", Content: "v2", Status: model.ArticlePublished, Version: stale,
	}); err != nil {
		assertConflict(t, err)
	} else {
		t.Fatal("归档后旧 version 的提交被接受了，状态会被改回已发布")
	}

	got, err := svc.GetMine(author.ID, roles, a.ID)
	if err != nil {
		t.Fatalf("get mine: %v", err)
	}
	if got.Status != model.ArticleArchived {
		t.Fatalf("状态 = %q，期望仍为 %q", got.Status, model.ArticleArchived)
	}
}

func TestArticleService_SearchKeyword(t *testing.T) {
	svc, db := newArticleService(t)
	author := createTestUser(t, db, "author", model.RoleAuthor)

	create := func(title, content string) {
		t.Helper()
		if _, err := svc.Create(author.ID, dto.ArticleRequest{Title: title, Content: content, Status: model.ArticlePublished}); err != nil {
			t.Fatalf("create %q: %v", title, err)
		}
	}
	create("Go 并发模式", "channel 与 goroutine")
	create("Rust 所有权", "borrow checker")

	cases := []struct {
		name    string
		keyword string
		want    int64
	}{
		{"命中标题", "并发", 1},
		{"命中正文", "borrow", 1},
		{"无命中", "python", 0},
		// `%` 若不转义会变成通配符，把「搜不到」变成「搜到全部」。
		{"通配符不匹配全部", "%", 0},
		{"下划线不匹配任意单字符", "_", 0},
		{"运算符关键词无命中", "+-*", 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, total, err := svc.ListPublished(1, 10, tc.keyword, "", "")
			if err != nil {
				t.Fatalf("search %q: %v", tc.keyword, err)
			}
			if total != tc.want {
				t.Fatalf("search %q: got %d, want %d", tc.keyword, total, tc.want)
			}
		})
	}
}
