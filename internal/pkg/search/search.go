// Package search 把用户输入的关键词转换成可安全用于 SQL 检索的形式。
//
// 关键词来自公开接口，不加处理直接使用会有两类问题：
//   - MySQL 全文检索的 BOOLEAN MODE 查询串里，`+ - > < ( ) ~ * "` 都是运算符，
//     用户搜 `a-b` 会被解析成「排除 b」，搜 `(` 直接语法错误；
//   - LIKE 模式里 `%` 与 `_` 是通配符，搜 `%` 会匹配到全部文章。
//
// 因此统一在这里做归一化与转义，仓储层不再关心关键词的原始形态。
package search

import (
	"strings"
	"unicode"
)

const (
	// maxTerms 限制参与检索的词数：关键词越长，全文索引要合并的倒排链越多，
	// 超出部分对相关性的贡献很低，却会把单次查询的成本抬得很高。
	maxTerms = 8
	// maxTermRunes 限制单个词的长度，避免超长输入被当作一个巨型 token 处理。
	maxTermRunes = 32
)

// Terms 归一化关键词并切分为词：只保留字母、数字与汉字（unicode 的 Letter 已涵盖 CJK），
// 其余字符（空格、标点、全文检索运算符）一律作为分隔符丢弃，同时去重、限长、限量。
// 返回空切片表示该关键词不包含任何可用于检索的内容。
func Terms(keyword string) []string {
	raw := strings.FieldsFunc(keyword, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	terms := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, t := range raw {
		t = strings.ToLower(truncate(t, maxTermRunes))
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		terms = append(terms, t)
		if len(terms) == maxTerms {
			break
		}
	}
	return terms
}

// BooleanQuery 由 Terms 的结果构造 BOOLEAN MODE 查询串。
// 每个词前缀 `+` 表示「必须出现」，即多词之间是 AND 语义——搜「Go 并发」
// 期望的是同时讲这两件事的文章，而不是任意命中其一的全部文章。
func BooleanQuery(terms []string) string {
	if len(terms) == 0 {
		return ""
	}
	var b strings.Builder
	for i, t := range terms {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteByte('+')
		b.WriteString(t)
	}
	return b.String()
}

// LikePattern 把关键词转成 LIKE 模式：转义 `\`、`%`、`_` 后包在前后通配符之间。
// 调用方必须配套使用 ESCAPE '\\'，否则转义符本身在 SQLite 上不生效。
func LikePattern(keyword string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(keyword) + "%"
}

// truncate 按字符（而非字节）截断，避免把多字节字符切成乱码。
func truncate(s string, maxRunes int) string {
	if len(s) <= maxRunes {
		// 字节数不超过字符上限时必然不超限，省掉一次计数。
		return s
	}
	n := 0
	for i := range s {
		if n == maxRunes {
			return s[:i]
		}
		n++
	}
	return s
}
