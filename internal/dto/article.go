package dto

import (
	"time"

	"goroutice/internal/model"
)

// ArticleRequest 创建/更新文章请求。
type ArticleRequest struct {
	Title      string   `json:"title" binding:"required,max=255"`
	Slug       string   `json:"slug" binding:"max=255"`
	Summary    string   `json:"summary" binding:"max=500"`
	Content    string   `json:"content" binding:"required,max=1048576"`
	CoverImage string   `json:"cover_image" binding:"max=255"`
	Status     string   `json:"status" binding:"omitempty,oneof=draft published archived"`
	CategoryID string   `json:"category_id"`
	TagIDs     []string `json:"tag_ids"`
	// Version 是乐观锁：把读取详情时拿到的 version 回传，服务端据此拒绝覆盖他人已提交的修改。
	// 传 0 或省略表示不做并发检查（兼容尚未接该字段的客户端）。
	Version int `json:"version" binding:"min=0"`
}

// ArticleStatusRequest 更新文章状态请求。
type ArticleStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=draft published archived"`
}

// ArticleFeatureRequest 更新文章置顶/推荐请求（指针区分是否设置）。
type ArticleFeatureRequest struct {
	IsPinned   *bool `json:"is_pinned"`
	IsFeatured *bool `json:"is_featured"`
}

// ArticleInfo 文章详情响应（含正文）。
type ArticleInfo struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Slug       string `json:"slug"`
	Summary    string `json:"summary"`
	Content    string `json:"content"`
	CoverImage string `json:"cover_image"`
	Status     string `json:"status"`
	ViewCount  int64  `json:"view_count"`
	// Version 用于乐观锁：编辑前先取此值，提交时原样回传（见 ArticleRequest.Version）。
	Version     int           `json:"version"`
	IsPinned    bool          `json:"is_pinned"`
	IsFeatured  bool          `json:"is_featured"`
	CategoryID  string        `json:"category_id"`
	Category    *CategoryInfo `json:"category,omitempty"`
	Tags        []TagInfo     `json:"tags,omitempty"`
	AuthorID    string        `json:"author_id"`
	Author      *UserInfo     `json:"author,omitempty"`
	PublishedAt *time.Time    `json:"published_at,omitempty"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

// ArticleSummary 文章列表响应（不含正文）。
type ArticleSummary struct {
	ID          string        `json:"id"`
	Title       string        `json:"title"`
	Slug        string        `json:"slug"`
	Summary     string        `json:"summary"`
	CoverImage  string        `json:"cover_image"`
	Status      string        `json:"status"`
	ViewCount   int64         `json:"view_count"`
	IsPinned    bool          `json:"is_pinned"`
	IsFeatured  bool          `json:"is_featured"`
	CategoryID  string        `json:"category_id"`
	Category    *CategoryInfo `json:"category,omitempty"`
	Tags        []TagInfo     `json:"tags,omitempty"`
	AuthorID    string        `json:"author_id"`
	Author      *UserInfo     `json:"author,omitempty"`
	PublishedAt *time.Time    `json:"published_at,omitempty"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

// ToArticleInfo 将文章模型转换为详情响应结构。
func ToArticleInfo(a *model.Article) *ArticleInfo {
	if a == nil {
		return nil
	}
	return &ArticleInfo{
		ID:          a.ID,
		Title:       a.Title,
		Slug:        a.Slug,
		Summary:     a.Summary,
		Content:     a.Content,
		CoverImage:  a.CoverImage,
		Status:      a.Status,
		ViewCount:   a.ViewCount,
		Version:     a.Version,
		IsPinned:    a.IsPinned,
		IsFeatured:  a.IsFeatured,
		CategoryID:  a.CategoryID,
		Category:    ToCategoryInfo(a.Category),
		Tags:        toTagInfos(a.Tags),
		AuthorID:    a.AuthorID,
		Author:      toAuthorInfo(a.Author),
		PublishedAt: a.PublishedAt,
		CreatedAt:   a.CreatedAt,
		UpdatedAt:   a.UpdatedAt,
	}
}

// ToArticleSummary 将文章模型转换为列表响应结构。
func ToArticleSummary(a *model.Article) *ArticleSummary {
	if a == nil {
		return nil
	}
	return &ArticleSummary{
		ID:          a.ID,
		Title:       a.Title,
		Slug:        a.Slug,
		Summary:     a.Summary,
		CoverImage:  a.CoverImage,
		Status:      a.Status,
		ViewCount:   a.ViewCount,
		IsPinned:    a.IsPinned,
		IsFeatured:  a.IsFeatured,
		CategoryID:  a.CategoryID,
		Category:    ToCategoryInfo(a.Category),
		Tags:        toTagInfos(a.Tags),
		AuthorID:    a.AuthorID,
		Author:      toAuthorInfo(a.Author),
		PublishedAt: a.PublishedAt,
		CreatedAt:   a.CreatedAt,
		UpdatedAt:   a.UpdatedAt,
	}
}

// toAuthorInfo 构造文章里的作者信息：清空邮箱。
// 文章接口既用于公开列表也用于管理端，作者邮箱属于账号 PII，
// 不应出现在任何无需登录的响应里（管理端需要邮箱时走 /admin/users）。
func toAuthorInfo(u *model.User) *UserInfo {
	info := ToUserInfo(u)
	if info == nil {
		return nil
	}
	info.Email = ""
	return info
}

// toTagInfos 将标签模型切片转换为响应结构，空切片返回 nil。
func toTagInfos(tags []model.Tag) []TagInfo {
	if len(tags) == 0 {
		return nil
	}
	out := make([]TagInfo, 0, len(tags))
	for i := range tags {
		out = append(out, *ToTagInfo(&tags[i]))
	}
	return out
}

// ArticleRevisionSummary 修订列表项（不含正文）。
type ArticleRevisionSummary struct {
	ID        string    `json:"id"`
	ArticleID string    `json:"article_id"`
	EditorID  string    `json:"editor_id"`
	Version   int       `json:"version"`
	Title     string    `json:"title"`
	Slug      string    `json:"slug"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// ArticleRevisionInfo 修订详情（含正文），字段与 ArticleRequest 对齐，可直接用于回溯。
type ArticleRevisionInfo struct {
	ArticleRevisionSummary
	Summary    string   `json:"summary"`
	Content    string   `json:"content"`
	CoverImage string   `json:"cover_image"`
	CategoryID string   `json:"category_id"`
	TagIDs     []string `json:"tag_ids"`
}

// ToArticleRevisionSummary 将修订模型转换为列表项。
func ToArticleRevisionSummary(r *model.ArticleRevision) *ArticleRevisionSummary {
	if r == nil {
		return nil
	}
	return &ArticleRevisionSummary{
		ID:        r.ID,
		ArticleID: r.ArticleID,
		EditorID:  r.EditorID,
		Version:   r.Version,
		Title:     r.Title,
		Slug:      r.Slug,
		Status:    r.Status,
		CreatedAt: r.CreatedAt,
	}
}

// ToArticleRevisionInfo 将修订模型转换为详情。
func ToArticleRevisionInfo(r *model.ArticleRevision) *ArticleRevisionInfo {
	if r == nil {
		return nil
	}
	return &ArticleRevisionInfo{
		ArticleRevisionSummary: *ToArticleRevisionSummary(r),
		Summary:                r.Summary,
		Content:                r.Content,
		CoverImage:             r.CoverImage,
		CategoryID:             r.CategoryID,
		TagIDs:                 r.TagIDList(),
	}
}
