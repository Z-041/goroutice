package model

// Tag 文章标签模型。
type Tag struct {
	Base
	Name string `gorm:"size:64;uniqueIndex;not null" json:"name"`
	Slug string `gorm:"size:64;uniqueIndex;not null" json:"slug"`
}
