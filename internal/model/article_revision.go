package model

import "strings"

// tagIDSeparator 是修订快照里标签 ID 的分隔符。
// 标签 ID 是 UUID，不含逗号，因此用逗号拼接不会产生歧义，
// 也就不必为了一份快照单独建一张关联表。
const tagIDSeparator = ","

// ArticleRevision 文章修订快照。
//
// 每次更新前把文章的可编辑状态整份存下来，因此「第 N 条修订」就等于
// 「文章的第 N 个版本」，当前内容则是第 max(version)+1 个版本。
// 存整份快照而不是增量 diff：文章正文是长文本，重放一串 diff 的失败模式
// 远比多存几份副本难排查，而博客的修订量并不大，并由 article.revision_keep 兜底。
type ArticleRevision struct {
	Base
	ArticleID string `gorm:"size:36;not null;index:idx_article_revision,priority:1" json:"article_id"`
	// Version 是文章维度的版本序号，从 1 递增。
	// 排序用它而不是 created_at：同一秒内的多次保存时间戳完全相同，排序结果不稳定。
	Version    int    `gorm:"not null;index:idx_article_revision,priority:2" json:"version"`
	EditorID   string `gorm:"size:36;index" json:"editor_id"`
	Title      string `gorm:"size:255;not null" json:"title"`
	Slug       string `gorm:"size:255;not null" json:"slug"`
	Summary    string `gorm:"size:500" json:"summary"`
	Content    string `gorm:"type:longtext" json:"content"`
	CoverImage string `gorm:"size:255" json:"cover_image"`
	Status     string `gorm:"size:16;not null" json:"status"`
	CategoryID string `gorm:"size:36" json:"category_id"`
	// TagIDs 是逗号分隔的标签 ID 列表，列宽与 UUID 数量上限相匹配。
	TagIDs string `gorm:"size:1024" json:"tag_ids"`
}

// TagIDList 把快照里的标签 ID 串拆成切片，空串返回 nil。
func (r *ArticleRevision) TagIDList() []string {
	if r.TagIDs == "" {
		return nil
	}
	return strings.Split(r.TagIDs, tagIDSeparator)
}

// SetTagIDs 把标签 ID 切片序列化为逗号分隔串，空切片写为空串。
func (r *ArticleRevision) SetTagIDs(ids []string) {
	r.TagIDs = strings.Join(ids, tagIDSeparator)
}

// SnapshotFrom 用文章当前状态填充修订快照中除版本号与编辑者以外的字段。
func (r *ArticleRevision) SnapshotFrom(a *Article) {
	r.ArticleID = a.ID
	r.Title = a.Title
	r.Slug = a.Slug
	r.Summary = a.Summary
	r.Content = a.Content
	r.CoverImage = a.CoverImage
	r.Status = a.Status
	r.CategoryID = a.CategoryID

	ids := make([]string, 0, len(a.Tags))
	for i := range a.Tags {
		ids = append(ids, a.Tags[i].ID)
	}
	r.SetTagIDs(ids)
}
