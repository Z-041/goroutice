package repository

import (
	"strings"
	"testing"

	"goroutice/internal/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// newDryRunDB 返回一个只构造 SQL、不真正执行的数据库会话。
// 用来断言查询的构造方式：SQLite 没有 FULLTEXT 语法，这些分支没法靠真跑查询来验证。
func newDryRunDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return db.Session(&gorm.Session{DryRun: true})
}

// buildKeywordSQL 施加关键词过滤并返回最终 SQL 与绑定参数。
func buildKeywordSQL(t *testing.T, repo *ArticleRepository, db *gorm.DB, keyword string) (string, []any) {
	t.Helper()

	q, searchable := repo.applyKeyword(db.Model(&model.Article{}), keyword)
	if !searchable {
		t.Fatalf("关键词 %q 被判定为不可检索", keyword)
	}

	var articles []model.Article
	stmt := q.Find(&articles).Statement
	return stmt.SQL.String(), stmt.Vars
}

func TestArticleRepository_FulltextSearch(t *testing.T) {
	db := newDryRunDB(t)
	repo := NewArticleRepository(db, true)

	sql, vars := buildKeywordSQL(t, repo, db, "Go 并发")

	if !strings.Contains(sql, "MATCH(title, summary, content) AGAINST") {
		t.Fatalf("全文检索 SQL 未使用 MATCH ... AGAINST: %s", sql)
	}
	if !strings.Contains(sql, "IN BOOLEAN MODE") {
		t.Fatalf("全文检索未使用布尔模式: %s", sql)
	}

	// MATCH 的列顺序必须与 database.articleFulltextColumns 一致，
	// 否则 MySQL 会以「找不到匹配的 FULLTEXT 索引」直接报错。
	// 绑定参数应当是 AND 语义的布尔查询串，且用户输入里的标点已被剥掉。
	if !hasVar(vars, "+go +并发") {
		t.Fatalf("布尔查询串不符合预期: %v", vars)
	}
}

func TestArticleRepository_FulltextSearchWithoutUsableTerms(t *testing.T) {
	db := newDryRunDB(t)
	repo := NewArticleRepository(db, true)

	// 全是运算符的关键词没有任何可检索内容，必须回退成「不施加过滤」并由调用方返回空集，
	// 而不是生成一个 `AGAINST('')` 的空查询。
	if _, searchable := repo.applyKeyword(db.Model(&model.Article{}), "+-*"); searchable {
		t.Fatal("纯运算符关键词不应被判定为可检索")
	}
}

func TestArticleRepository_LikeSearchEscapesWildcards(t *testing.T) {
	db := newDryRunDB(t)
	repo := NewArticleRepository(db, false)

	sql, vars := buildKeywordSQL(t, repo, db, "100%_x")

	if !strings.Contains(sql, "ESCAPE") {
		t.Fatalf("LIKE 检索缺少 ESCAPE 子句: %s", sql)
	}
	// `%` 与 `_` 必须被转义，否则搜 `%` 会匹配到全部文章。
	if !hasVar(vars, `%100\%\_x%`) {
		t.Fatalf("LIKE 模式未正确转义: %v", vars)
	}
}

func hasVar(vars []any, want string) bool {
	for _, v := range vars {
		if s, ok := v.(string); ok && s == want {
			return true
		}
	}
	return false
}
