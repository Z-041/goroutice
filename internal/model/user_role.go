package model

// UserRole 用户与角色的多对多关联，一个用户可拥有多个角色。
type UserRole struct {
	ID     uint   `gorm:"primaryKey" json:"id"`
	UserID string `gorm:"size:36;not null;uniqueIndex:idx_user_role,priority:1" json:"user_id"`
	Role   string `gorm:"size:16;not null;uniqueIndex:idx_user_role,priority:2" json:"role"`
}
