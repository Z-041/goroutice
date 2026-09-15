package model

// Category 文章分类模型。
type Category struct {
	Base
	Name        string `gorm:"size:64;uniqueIndex;not null" json:"name"`
	Slug        string `gorm:"size:64;uniqueIndex;not null" json:"slug"`
	Description string `gorm:"size:255" json:"description"`
	Sort        int    `gorm:"not null;default:0" json:"sort"`
}
