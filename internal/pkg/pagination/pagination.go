package pagination

import "strconv"

const (
	// DefaultPage 默认页码。
	DefaultPage = 1
	// DefaultSize 默认每页条数。
	DefaultSize = 10
	// MaxSize 单页最大条数。
	MaxSize = 100
	// MaxPage 最大页码，防止 (page-1)*size 溢出为负数导致 SQL 偏移量非法。
	MaxPage = 100000
)

// Query 分页参数。
type Query struct {
	Page int
	Size int
}

// Parse 从字符串解析分页参数，非法值回退为默认值。
func Parse(pageStr, sizeStr string) Query {
	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = DefaultPage
	}
	if page > MaxPage {
		page = MaxPage
	}

	size, err := strconv.Atoi(sizeStr)
	if err != nil || size < 1 {
		size = DefaultSize
	}
	if size > MaxSize {
		size = MaxSize
	}

	return Query{Page: page, Size: size}
}

// Offset 计算 SQL 偏移量。
func (q Query) Offset() int {
	return (q.Page - 1) * q.Size
}
