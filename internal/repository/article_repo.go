package repository

import (
	"errors"
	"time"

	"goroutice/internal/model"
	"goroutice/internal/pkg/search"

	"gorm.io/gorm"
)

// ArticleFilter 文章列表过滤条件。
type ArticleFilter struct {
	Keyword    string
	CategoryID string
	TagID      string
	AuthorID   string
	Status     string
}

// ArticleRepository 文章数据访问。
type ArticleRepository struct {
	db *gorm.DB
	// fulltext 表示数据库已具备文章全文索引，关键词检索走 MATCH ... AGAINST；
	// 否则退回 LIKE。是否可用由 database.EnsureArticleFulltextIndex 在启动时探测后注入：
	// 驱动是 MySQL 但没有索引时，MATCH 会直接报错而不是降级，因此不能自行按驱动名判断。
	fulltext bool
}

// NewArticleRepository 构造 ArticleRepository，fulltext 表示全文索引是否可用。
func NewArticleRepository(db *gorm.DB, fulltext bool) *ArticleRepository {
	return &ArticleRepository{db: db, fulltext: fulltext}
}

// Create 在事务中创建文章并绑定标签。
func (r *ArticleRepository) Create(a *model.Article, tagIDs []string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(a).Error; err != nil {
			return err
		}
		if len(tagIDs) > 0 {
			if err := replaceTags(tx, a, tagIDs); err != nil {
				return err
			}
		}
		return nil
	})
}

// ArticleUpdate 是一次文章内容更新要落库的全部内容。
type ArticleUpdate struct {
	Article *model.Article
	TagIDs  []string
	// Revision 非空时在同一事务内追加一条修订快照。
	Revision *model.ArticleRevision
	// KeepRevisions 是保留的修订条数，<= 0 表示不裁剪。
	KeepRevisions int
	// ExpectedVersion > 0 时启用乐观锁：仅当库中 version 与它相等才写入，
	// 否则返回 ErrVersionConflict。传 0 表示不做并发检查。
	ExpectedVersion int
}

// ErrVersionConflict 表示文章在客户端读取之后已被他人修改，本次写入被拒绝。
// 由服务层翻译为 409，而不是 500：这不是服务端故障，是调用方该重新读取后再提交。
var ErrVersionConflict = errors.New("article version conflict")

// Update 在事务中更新文章可变字段并重建标签关联。
// 只写入白名单列，避免全字段回写覆盖并发自增的 view_count 与不可变的 created_at。
//
// u.Revision 非空时在同一事务内追加一条修订快照，并把该文章的历史裁剪到 u.KeepRevisions 条以内。
// 快照与正文更新必须同事务：分两次写会出现「正文已改但历史没记」的静默丢档，
// 而丢档恰恰是修订功能唯一要避免的事。
func (r *ArticleRepository) Update(u ArticleUpdate) error {
	a := u.Article

	return r.db.Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{
			"title":        a.Title,
			"slug":         a.Slug,
			"summary":      a.Summary,
			"content":      a.Content,
			"cover_image":  a.CoverImage,
			"status":       a.Status,
			"category_id":  a.CategoryID,
			"published_at": a.PublishedAt,
			"updated_at":   time.Now(),
			// 版本号在 SQL 里自增，不能读出来 +1 再写回：并发下的写回会丢掉其中一次自增。
			"version": gorm.Expr("version + 1"),
		}

		q := tx.Model(&model.Article{}).Where("id = ?", a.ID)
		if u.ExpectedVersion > 0 {
			q = q.Where("version = ?", u.ExpectedVersion)
		}
		res := q.Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		// 匹配不到行说明版本已被他人推进。这里不会把「字段值没变化」误判成冲突：
		// MySQL 默认返回的是「实际变更行数」，而 version 每次都会变，命中即必然计数。
		if u.ExpectedVersion > 0 && res.RowsAffected == 0 {
			return ErrVersionConflict
		}

		if err := replaceTags(tx, a, u.TagIDs); err != nil {
			return err
		}
		if u.Revision == nil {
			return nil
		}
		return appendRevision(tx, u.Revision, u.KeepRevisions)
	})
}

// appendRevision 在事务内追加一条修订快照并裁剪历史。
// 版本号取「当前最大版本 + 1」而不是在应用层缓存计数：并发保存同一篇文章时，
// 应用层计数会算出重复版本号，而这里由数据库的行锁串行化。
func appendRevision(tx *gorm.DB, rev *model.ArticleRevision, keepRevisions int) error {
	var maxVersion int
	if err := tx.Model(&model.ArticleRevision{}).
		Where("article_id = ?", rev.ArticleID).
		Select("COALESCE(MAX(version), 0)").
		Scan(&maxVersion).Error; err != nil {
		return err
	}
	rev.Version = maxVersion + 1
	if err := tx.Create(rev).Error; err != nil {
		return err
	}
	if keepRevisions <= 0 {
		return nil
	}
	// 只保留最新的 keepRevisions 条，即版本号大于 max-keep 的那些。
	return tx.Where("article_id = ? AND version <= ?", rev.ArticleID, rev.Version-keepRevisions).
		Delete(&model.ArticleRevision{}).Error
}

// Delete 软删除文章，并清理标签关联与修订记录。
// 关联表与修订表都没有软删除列，不清理会在表里长期堆积指向已删除文章的孤儿行。
func (r *ArticleRepository) Delete(id string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM article_tags WHERE article_id = ?", id).Error; err != nil {
			return err
		}
		// 文章被删除后不再提供回溯入口，历史修订留着既无用途又要一直占用正文副本。
		if err := tx.Exec("DELETE FROM article_revisions WHERE article_id = ?", id).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&model.Article{}).Error
	})
}

// GetByID 按 ID 查询文章并预加载关联。
func (r *ArticleRepository) GetByID(id string) (*model.Article, error) {
	var a model.Article
	err := r.db.Preload("Author").Preload("Category").Preload("Tags").Where("id = ?", id).First(&a).Error
	return &a, err
}

// GetBySlug 按 slug 查询文章并预加载关联。
func (r *ArticleRepository) GetBySlug(slug string) (*model.Article, error) {
	var a model.Article
	err := r.db.Preload("Author").Preload("Category").Preload("Tags").
		Where("slug = ?", slug).First(&a).Error
	return &a, err
}

// List 分页查询文章。
func (r *ArticleRepository) List(page, size int, f ArticleFilter) ([]model.Article, int64, error) {
	q := r.db.Model(&model.Article{}).Preload("Author").Preload("Category").Preload("Tags")

	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.CategoryID != "" {
		q = q.Where("category_id = ?", f.CategoryID)
	}
	if f.AuthorID != "" {
		q = q.Where("author_id = ?", f.AuthorID)
	}
	if f.Keyword != "" {
		var searchable bool
		if q, searchable = r.applyKeyword(q, f.Keyword); !searchable {
			// 关键词里没有任何可检索的字符（例如全是标点）。返回空集而不是跳过过滤：
			// 后者会让一次无效搜索看起来像「搜到了全部文章」，比返回空更让人困惑。
			return []model.Article{}, 0, nil
		}
	}
	if f.TagID != "" {
		sub := r.db.Table("article_tags").Select("article_id").Where("tag_id = ?", f.TagID)
		q = q.Where("id IN (?)", sub)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var articles []model.Article
	err := q.Order("is_pinned DESC, is_featured DESC, created_at DESC").Offset((page - 1) * size).Limit(size).Find(&articles).Error
	return articles, total, err
}

// applyKeyword 对查询施加关键词过滤，第二个返回值表示该关键词是否可用于检索。
//
// 两条路径的语义差异是有意为之：全文索引走「按词 AND」，LIKE 走「整串子串匹配」。
// 前者才是检索该有的样子（搜「Go 并发」不该返回只提到 Go 的文章），
// 但 SQLite 没有 FULLTEXT，测试环境只能退回子串匹配。
func (r *ArticleRepository) applyKeyword(q *gorm.DB, keyword string) (*gorm.DB, bool) {
	if r.fulltext {
		query := search.BooleanQuery(search.Terms(keyword))
		if query == "" {
			return q, false
		}
		return q.Where("MATCH(title, summary, content) AGAINST (? IN BOOLEAN MODE)", query), true
	}

	pattern := search.LikePattern(keyword)
	// ESCAPE 必须显式声明：SQLite 没有默认转义符，靠 MySQL 的 `\` 默认行为会导致
	// 同一个关键词在两个环境下语义不同。
	return q.Where(
		`(title LIKE ? ESCAPE '\' OR summary LIKE ? ESCAPE '\' OR content LIKE ? ESCAPE '\')`,
		pattern, pattern, pattern,
	), true
}

// IncrementView 文章浏览数自增。
func (r *ArticleRepository) IncrementView(id string) error {
	return r.db.Model(&model.Article{}).Where("id = ?", id).
		UpdateColumn("view_count", gorm.Expr("view_count + ?", 1)).Error
}

// UpdateStatus 更新文章状态；发布时间只在首次发布时写入，重复发布不覆盖原值。
//
// status 是 ArticleRequest 能覆盖的字段，因此这里也要推进 version：
// 否则管理员归档文章后，作者用旧 version 提交的 PUT 会毫无察觉地把状态改回去。
func (r *ArticleRepository) UpdateStatus(id string, status string) error {
	updates := map[string]interface{}{
		"status":  status,
		"version": gorm.Expr("version + 1"),
	}
	if status == model.ArticlePublished {
		updates["published_at"] = gorm.Expr("COALESCE(published_at, ?)", time.Now())
	}
	return r.db.Model(&model.Article{}).Where("id = ?", id).Updates(updates).Error
}

// UpdateFeature 按需更新文章置顶/推荐标记，nil 表示保持原值，避免读-改-写丢失并发更新。
//
// 与 UpdateStatus 不同，这里不推进 version：is_pinned / is_featured 不在 ArticleRequest 里，
// PUT 覆盖不到它们，推进版本只会让正在编辑的作者收到一次无从处理的 409。
func (r *ArticleRepository) UpdateFeature(id string, isPinned, isFeatured *bool) error {
	updates := map[string]interface{}{}
	if isPinned != nil {
		updates["is_pinned"] = *isPinned
	}
	if isFeatured != nil {
		updates["is_featured"] = *isFeatured
	}
	if len(updates) == 0 {
		return nil
	}
	return r.db.Model(&model.Article{}).Where("id = ?", id).Updates(updates).Error
}

func replaceTags(tx *gorm.DB, a *model.Article, tagIDs []string) error {
	var tags []model.Tag
	if len(tagIDs) > 0 {
		if err := tx.Where("id IN ?", tagIDs).Find(&tags).Error; err != nil {
			return err
		}
	}
	return tx.Model(a).Association("Tags").Replace(tags)
}
