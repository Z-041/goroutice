package search

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTerms(t *testing.T) {
	cases := []struct {
		name    string
		keyword string
		want    []string
	}{
		{"按标点切分", "Hello, World!", []string{"hello", "world"}},
		{"保留中文", "Go 并发", []string{"go", "并发"}},
		{"去重", "go Go GO", []string{"go"}},
		{"全是运算符时无词可用", "+-*\"()~", nil},
		{"通配符不参与检索", "100%", []string{"100"}},
		{"空白关键词", "   ", nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Terms(tc.keyword)
			if len(got) != len(tc.want) {
				t.Fatalf("Terms(%q) = %v, want %v", tc.keyword, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("Terms(%q) = %v, want %v", tc.keyword, got, tc.want)
				}
			}
		})
	}
}

func TestTerms_Limits(t *testing.T) {
	// 词数超过上限时截断，避免一次检索要合并过多倒排链。
	many := "a b c d e f g h i j k"
	if got := Terms(many); len(got) != maxTerms {
		t.Fatalf("Terms(%q) 词数 = %d, want %d", many, len(got), maxTerms)
	}

	// 单个超长词被截断到 maxTermRunes。
	long := strings.Repeat("x", maxTermRunes*2)
	got := Terms(long)
	if len(got) != 1 || len(got[0]) != maxTermRunes {
		t.Fatalf("Terms(超长词) = %v (len=%d), want 单个长度 %d 的词", got, len(got), maxTermRunes)
	}

	// 按字符截断，不能把多字节字符切成乱码：40 个汉字截到 32 个仍然是合法 UTF-8。
	cjk := strings.Repeat("字", maxTermRunes+8)
	got = Terms(cjk)
	if len(got) != 1 || !utf8.ValidString(got[0]) || utf8.RuneCountInString(got[0]) != maxTermRunes {
		t.Fatalf("Terms(超长中文) 结果非法: %q", got)
	}
}

func TestBooleanQuery(t *testing.T) {
	if got := BooleanQuery(Terms("Go 并发")); got != "+go +并发" {
		t.Fatalf("BooleanQuery = %q, want %q", got, "+go +并发")
	}
	if got := BooleanQuery(nil); got != "" {
		t.Fatalf("BooleanQuery(nil) = %q, want 空串", got)
	}
}

func TestLikePattern(t *testing.T) {
	cases := []struct {
		keyword string
		want    string
	}{
		// 不转义的话 `%` 会变成通配符，搜 `100%` 会匹配到全部文章。
		{"100%", `%100\%%`},
		{"a_b", `%a\_b%`},
		{`c:\path`, `%c:\\path%`},
		{"正常", "%正常%"},
	}

	for _, tc := range cases {
		if got := LikePattern(tc.keyword); got != tc.want {
			t.Fatalf("LikePattern(%q) = %q, want %q", tc.keyword, got, tc.want)
		}
	}
}
