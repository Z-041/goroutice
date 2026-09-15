package slug

import (
	"strings"

	"github.com/google/uuid"
)

// Make 将文本转换为 URL 友好的 slug。
// 仅保留小写字母、数字与连字符；若结果为空（如纯中文标题），则回退为随机串。
func Make(text string) string {
	s := strings.ToLower(strings.TrimSpace(text))

	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == ' ' || r == '-':
			if !lastDash {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}

	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = uuid.NewString()[:8]
	}
	return out
}

// Validate 校验用户提供的 slug 是否合法：仅小写字母、数字与连字符，
// 且不能以连字符开头或结尾，长度不超过 255。
func Validate(s string) bool {
	if s == "" || len(s) > 255 {
		return false
	}
	if s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		isLower := c >= 'a' && c <= 'z'
		isDigit := c >= '0' && c <= '9'
		if !isLower && !isDigit && c != '-' {
			return false
		}
	}
	return true
}
