package service

import (
	"errors"
	"slices"
	"time"

	"goroutice/internal/dto"
	"goroutice/internal/model"
	"goroutice/internal/pkg/apperror"
	"goroutice/internal/pkg/slug"
	"goroutice/internal/repository"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ArticleService 文章服务。
type ArticleService struct {
	articleRepo  *repository.ArticleRepository
	categoryRepo *repository.CategoryRepository
	tagRepo      *repository.TagRepository
	revisionRepo *repository.ArticleRevisionRepository
	// revisionKeep 是每篇文章保留的历史修订条数，<= 0 表示不记录修订。
	revisionKeep int
	// views 负责浏览量的来源去重：没有它，刷新页面和爬虫都会把 view_count 灌成无意义的数字。
	views *ViewTracker
}

// NewArticleService 构造 ArticleService，revisionKeep 为每篇文章保留的修订条数，views 为浏览量去重器。
func NewArticleService(
	articleRepo *repository.ArticleRepository,
	categoryRepo *repository.CategoryRepository,
	tagRepo *repository.TagRepository,
	revisionRepo *repository.ArticleRevisionRepository,
	revisionKeep int,
	views *ViewTracker,
) *ArticleService {
	return &ArticleService{
		articleRepo:  articleRepo,
		categoryRepo: categoryRepo,
		tagRepo:      tagRepo,
		revisionRepo: revisionRepo,
		revisionKeep: revisionKeep,
		views:        views,
	}
}

// Create 创建文章。
func (s *ArticleService) Create(authorID string, req dto.ArticleRequest) (*dto.ArticleInfo, error) {
	if req.CategoryID != "" {
		if _, err := s.categoryRepo.GetByID(req.CategoryID); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, apperror.BadRequest("category not found")
			}
			return nil, err
		}
	}

	tagIDs := uniqueStrings(req.TagIDs)
	if err := s.validateTagIDs(tagIDs); err != nil {
		return nil, err
	}

	base := req.Slug
	if base == "" {
		base = slug.Make(req.Title)
	} else if !slug.Validate(base) {
		return nil, apperror.BadRequest("invalid slug")
	}
	slugStr, err := resolveSlug(base, articleSlugMaxLen, func(slug string) (bool, error) {
		_, err := s.articleRepo.GetBySlug(slug)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}

	status := req.Status
	if status == "" {
		status = model.ArticleDraft
	}

	a := &model.Article{
		Title:      req.Title,
		Slug:       slugStr,
		Summary:    req.Summary,
		Content:    req.Content,
		CoverImage: req.CoverImage,
		Status:     status,
		AuthorID:   authorID,
		CategoryID: req.CategoryID,
	}
	if status == model.ArticlePublished {
		now := time.Now()
		a.PublishedAt = &now
	}

	if err := s.articleRepo.Create(a, tagIDs); err != nil {
		// slug 先查后写存在竞争窗口，唯一索引是最终防线：翻译成 409 而不是 500。
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, apperror.Conflict("slug already exists")
		}
		return nil, err
	}
	return s.getByID(a.ID)
}

// Update 更新文章，仅作者本人或管理员可操作。
// req.Version 非 0 时启用乐观锁：与库中版本不符说明文章已被他人改过，返回 409 而不是覆盖。
func (s *ArticleService) Update(userID string, roles []string, id string, req dto.ArticleRequest) (*dto.ArticleInfo, error) {
	a, err := s.loadForWrite(userID, roles, id)
	if err != nil {
		return nil, err
	}
	return s.applyUpdate(a, userID, req, req.Version)
}

// RestoreRevision 把文章回滚到指定修订，仅作者本人或管理员可操作。
// 回滚前会先按当前内容写一条修订，因此回滚本身同样可以被再回滚。
//
// 请求没有请求体，乐观锁用服务端刚读到的版本号：能拦住「读取之后、写入之前」被他人改动的情况，
// 调用方收到 409 后重新读取再回滚即可（回滚本身是幂等的）。
func (s *ArticleService) RestoreRevision(userID string, roles []string, articleID, revisionID string) (*dto.ArticleInfo, error) {
	a, err := s.loadForWrite(userID, roles, articleID)
	if err != nil {
		return nil, err
	}

	rev, err := s.loadOwnedRevision(articleID, revisionID)
	if err != nil {
		return nil, err
	}

	return s.applyUpdate(a, userID, dto.ArticleRequest{
		Title:      rev.Title,
		Slug:       rev.Slug,
		Summary:    rev.Summary,
		Content:    rev.Content,
		CoverImage: rev.CoverImage,
		Status:     rev.Status,
		CategoryID: rev.CategoryID,
		TagIDs:     rev.TagIDList(),
	}, a.Version)
}

// ListRevisions 分页查询某篇文章的修订历史，仅作者本人或管理员可操作。
func (s *ArticleService) ListRevisions(userID string, roles []string, articleID string, page, size int) ([]dto.ArticleRevisionSummary, int64, error) {
	if _, err := s.loadForWrite(userID, roles, articleID); err != nil {
		return nil, 0, err
	}

	revisions, total, err := s.revisionRepo.List(articleID, page, size)
	if err != nil {
		return nil, 0, err
	}
	out := make([]dto.ArticleRevisionSummary, 0, len(revisions))
	for i := range revisions {
		out = append(out, *dto.ToArticleRevisionSummary(&revisions[i]))
	}
	return out, total, nil
}

// GetRevision 获取单条修订详情（含正文），仅作者本人或管理员可操作。
func (s *ArticleService) GetRevision(userID string, roles []string, articleID, revisionID string) (*dto.ArticleRevisionInfo, error) {
	if _, err := s.loadForWrite(userID, roles, articleID); err != nil {
		return nil, err
	}
	rev, err := s.loadOwnedRevision(articleID, revisionID)
	if err != nil {
		return nil, err
	}
	return dto.ToArticleRevisionInfo(rev), nil
}

// loadForWrite 载入文章并校验写权限（作者本人或管理员）。
func (s *ArticleService) loadForWrite(userID string, roles []string, id string) (*model.Article, error) {
	a, err := s.articleRepo.GetByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.NotFound("article not found")
		}
		return nil, err
	}
	if !slices.Contains(roles, model.RoleAdmin) && a.AuthorID != userID {
		return nil, apperror.Forbidden("you can only modify your own articles")
	}
	return a, nil
}

// loadOwnedRevision 载入修订并确认它属于指定文章。
// 归属校验不能省：只按修订 ID 查询的话，知道任意修订 ID 就能把它的内容灌进自己的文章。
func (s *ArticleService) loadOwnedRevision(articleID, revisionID string) (*model.ArticleRevision, error) {
	rev, err := s.revisionRepo.GetByID(revisionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.NotFound("revision not found")
		}
		return nil, err
	}
	if rev.ArticleID != articleID {
		return nil, apperror.NotFound("revision not found")
	}
	return rev, nil
}

// applyUpdate 校验并施加变更，把「更新前」的状态存为一条修订后落库。
// expectedVersion > 0 时启用乐观锁，冲突由仓储层返回 ErrVersionConflict，这里翻译成 409。
func (s *ArticleService) applyUpdate(a *model.Article, editorID string, req dto.ArticleRequest, expectedVersion int) (*dto.ArticleInfo, error) {
	// 快照必须在下面就地改写 a 之前抓取，否则记下来的就是更新后的内容，
	// 修订历史会退化成「保存即等于当前版本」，失去回溯意义。
	rev := s.newRevision(a, editorID)

	var err error
	if a.CategoryID, err = s.resolveCategory(req.CategoryID); err != nil {
		return nil, err
	}

	tagIDs := uniqueStrings(req.TagIDs)
	if err := s.validateTagIDs(tagIDs); err != nil {
		return nil, err
	}

	a.Title = req.Title
	a.Summary = req.Summary
	a.Content = req.Content
	a.CoverImage = req.CoverImage
	s.applyStatus(a, req.Status)

	if err := s.resolveSlugForUpdate(a, req.Slug, a.ID); err != nil {
		return nil, err
	}

	// 修订与正文必须同一个事务：任一失败都要整体回滚，否则会留下「改了正文却没记快照」的静默丢档。
	err = s.articleRepo.Update(repository.ArticleUpdate{
		Article:         a,
		TagIDs:          tagIDs,
		Revision:        rev,
		KeepRevisions:   s.revisionKeep,
		ExpectedVersion: expectedVersion,
	})
	if errors.Is(err, repository.ErrVersionConflict) {
		return nil, apperror.Conflict("article was modified by someone else, reload and retry")
	}
	if err != nil {
		return nil, err
	}
	return s.getByID(a.ID)
}

// newRevision 抓取文章当前状态的修订快照；未启用修订（revisionKeep <= 0）时返回 nil。
func (s *ArticleService) newRevision(a *model.Article, editorID string) *model.ArticleRevision {
	if s.revisionKeep <= 0 {
		return nil
	}
	rev := &model.ArticleRevision{EditorID: editorID}
	rev.SnapshotFrom(a)
	return rev
}

// resolveCategory 校验分类存在并返回其 ID，空值返回空串。
func (s *ArticleService) resolveCategory(categoryID string) (string, error) {
	if categoryID == "" {
		return "", nil
	}
	if _, err := s.categoryRepo.GetByID(categoryID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", apperror.BadRequest("category not found")
		}
		return "", err
	}
	return categoryID, nil
}

// applyStatus 更新文章状态，并在首次发布时写入发布时间。
func (s *ArticleService) applyStatus(a *model.Article, status string) {
	if status == "" {
		return
	}
	// 与 ArticleRepository.UpdateStatus 的 COALESCE 语义保持一致：发布时间只写一次，
	// 归档后再次发布不应把原始发布时间刷新成当前时间。
	if status == model.ArticlePublished && a.PublishedAt == nil {
		now := time.Now()
		a.PublishedAt = &now
	}
	a.Status = status
}

// resolveSlugForUpdate 当请求携带新的 slug 时解析唯一 slug 并更新。
func (s *ArticleService) resolveSlugForUpdate(a *model.Article, slugStr, id string) error {
	if slugStr == "" || slugStr == a.Slug {
		return nil
	}
	if !slug.Validate(slugStr) {
		return apperror.BadRequest("invalid slug")
	}
	resolved, err := resolveSlug(slugStr, articleSlugMaxLen, func(slug string) (bool, error) {
		existing, err := s.articleRepo.GetBySlug(slug)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return existing.ID != id, nil
	})
	if err != nil {
		return err
	}
	a.Slug = resolved
	return nil
}

// Delete 删除文章，仅作者本人或管理员可操作。
func (s *ArticleService) Delete(userID string, roles []string, id string) error {
	a, err := s.articleRepo.GetByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperror.NotFound("article not found")
		}
		return err
	}
	if !slices.Contains(roles, model.RoleAdmin) && a.AuthorID != userID {
		return apperror.Forbidden("you can only delete your own articles")
	}
	return s.articleRepo.Delete(id)
}

// GetPublished 获取已发布文章详情（按 ID 或 slug），并按来源去重后累加浏览数。
//
// viewer 是访问者标识（由 handler 用 IP + User-Agent 拼成）：同一来源在窗口内重复打开
// 同一篇文章只计一次。不做去重的话，刷新一次算一次、爬虫抓一次算一次，view_count 会彻底失真。
func (s *ArticleService) GetPublished(key, viewer string) (*dto.ArticleInfo, error) {
	var (
		a   *model.Article
		err error
	)
	if _, parseErr := uuid.Parse(key); parseErr == nil {
		a, err = s.articleRepo.GetByID(key)
	} else {
		a, err = s.articleRepo.GetBySlug(key)
	}
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.NotFound("article not found")
		}
		return nil, err
	}
	if a.Status != model.ArticlePublished {
		return nil, apperror.NotFound("article not found")
	}

	if s.views.Fresh(a.ID, viewer) {
		_ = s.articleRepo.IncrementView(a.ID)
		a.ViewCount++
	}

	return dto.ToArticleInfo(a), nil
}

// ListPublished 分页查询已发布文章。
func (s *ArticleService) ListPublished(page, size int, keyword string, categoryID, tagID string) ([]dto.ArticleSummary, int64, error) {
	articles, total, err := s.articleRepo.List(page, size, repository.ArticleFilter{
		Keyword:    keyword,
		CategoryID: categoryID,
		TagID:      tagID,
		Status:     model.ArticlePublished,
	})
	if err != nil {
		return nil, 0, err
	}
	return toArticleSummaries(articles), total, nil
}

// ListMine 分页查询当前作者自己的文章（含草稿）。
func (s *ArticleService) ListMine(userID string, page, size int, status string) ([]dto.ArticleSummary, int64, error) {
	articles, total, err := s.articleRepo.List(page, size, repository.ArticleFilter{
		AuthorID: userID,
		Status:   status,
	})
	if err != nil {
		return nil, 0, err
	}
	return toArticleSummaries(articles), total, nil
}

// ListAdmin 管理员分页查询所有文章。
func (s *ArticleService) ListAdmin(page, size int, keyword, status string, categoryID string) ([]dto.ArticleSummary, int64, error) {
	articles, total, err := s.articleRepo.List(page, size, repository.ArticleFilter{
		Keyword:    keyword,
		Status:     status,
		CategoryID: categoryID,
	})
	if err != nil {
		return nil, 0, err
	}
	return toArticleSummaries(articles), total, nil
}

// GetMine 获取当前作者自己的文章详情（任意状态）。
func (s *ArticleService) GetMine(userID string, roles []string, id string) (*dto.ArticleInfo, error) {
	a, err := s.articleRepo.GetByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.NotFound("article not found")
		}
		return nil, err
	}
	if !slices.Contains(roles, model.RoleAdmin) && a.AuthorID != userID {
		return nil, apperror.Forbidden("you can only view your own articles")
	}
	return dto.ToArticleInfo(a), nil
}

// UpdateStatus 更新文章状态。
func (s *ArticleService) UpdateStatus(id string, status string) error {
	if status != model.ArticleDraft && status != model.ArticlePublished && status != model.ArticleArchived {
		return apperror.BadRequest("invalid status")
	}
	if _, err := s.articleRepo.GetByID(id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperror.NotFound("article not found")
		}
		return err
	}
	return s.articleRepo.UpdateStatus(id, status)
}

// SetFeature 更新文章置顶/推荐标记，未提供的字段保持原值。
func (s *ArticleService) SetFeature(id string, req dto.ArticleFeatureRequest) error {
	if req.IsPinned == nil && req.IsFeatured == nil {
		return apperror.BadRequest("nothing to update")
	}

	if _, err := s.articleRepo.GetByID(id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperror.NotFound("article not found")
		}
		return err
	}

	// 直接把指针透传给仓储层按需更新，避免先读后写覆盖并发修改的另一字段。
	return s.articleRepo.UpdateFeature(id, req.IsPinned, req.IsFeatured)
}

func (s *ArticleService) getByID(id string) (*dto.ArticleInfo, error) {
	a, err := s.articleRepo.GetByID(id)
	if err != nil {
		return nil, err
	}
	return dto.ToArticleInfo(a), nil
}

func toArticleSummaries(articles []model.Article) []dto.ArticleSummary {
	out := make([]dto.ArticleSummary, 0, len(articles))
	for i := range articles {
		out = append(out, *dto.ToArticleSummary(&articles[i]))
	}
	return out
}

// validateTagIDs 校验标签 ID 是否全部存在。
func (s *ArticleService) validateTagIDs(tagIDs []string) error {
	if len(tagIDs) == 0 {
		return nil
	}
	n, err := s.tagRepo.CountByIDs(tagIDs)
	if err != nil {
		return err
	}
	if n != int64(len(tagIDs)) {
		return apperror.BadRequest("one or more tags not found")
	}
	return nil
}

// uniqueStrings 去重并过滤空字符串。
func uniqueStrings(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	return out
}
