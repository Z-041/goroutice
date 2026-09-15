package service

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"goroutice/internal/pkg/apperror"
	"goroutice/internal/pkg/slug"
)

const (
	// taxonomySlugMaxLen 是分类/标签 slug 的列宽，与 model.Category/model.Tag 的 size:64 一致。
	taxonomySlugMaxLen = 64
	// articleSlugMaxLen 是文章 slug 的列宽，与 model.Article 的 size:255 一致。
	articleSlugMaxLen = 255
)

// slugExistsFunc 判断 slug 是否已存在。
type slugExistsFunc func(slug string) (bool, error)

// resolveSlug 保证 slug 唯一：若候选已存在，则追加 -2、-3 等后缀。
// maxLen 为目标列宽：追加后缀时截断 base，否则超长会在写库时报错并返回 500。
func resolveSlug(base string, maxLen int, exists slugExistsFunc) (string, error) {
	candidate := base
	for i := 2; ; i++ {
		ok, err := exists(candidate)
		if err != nil {
			return "", err
		}
		if !ok {
			return candidate, nil
		}
		suffix := fmt.Sprintf("-%d", i)
		candidate = truncateSlug(base, maxLen-len(suffix)) + suffix
	}
}

// taxonomySlug 解析分类/标签的 slug：未提供时由名称生成并截断到列宽，提供时校验合法性与长度。
func taxonomySlug(name, provided string) (string, error) {
	if provided == "" {
		return truncateSlug(slug.Make(name), taxonomySlugMaxLen), nil
	}
	if !slug.Validate(provided) {
		return "", apperror.BadRequest("invalid slug")
	}
	if len(provided) > taxonomySlugMaxLen {
		return "", apperror.BadRequest("slug is too long")
	}
	return provided, nil
}

// truncateSlug 把 slug 截断到不超过 maxLen，并去掉截断处残留的连字符。
func truncateSlug(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if len(s) > maxLen {
		s = s[:maxLen]
	}
	return strings.TrimRight(s, "-")
}

// truncateRunes 把字符串截断到最多 maxLen 个字符。
// 与 MySQL 的 varchar 长度语义一致（按字符而非字节计），且不会把多字节字符切成乱码。
func truncateRunes(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	n := 0
	for i := range s {
		if n == maxLen {
			return s[:i]
		}
		n++
	}
	return s
}
