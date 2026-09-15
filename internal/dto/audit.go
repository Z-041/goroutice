package dto

import (
	"time"

	"goroutice/internal/model"
)

// AuditInfo 权限审计记录响应。
type AuditInfo struct {
	ID               string    `json:"id"`
	OperatorID       string    `json:"operator_id"`
	OperatorUsername string    `json:"operator_username"`
	Action           string    `json:"action"`
	Detail           string    `json:"detail"`
	CreatedAt        time.Time `json:"created_at"`
}

// ToAuditInfo 将审计模型转换为响应结构。
func ToAuditInfo(a *model.PermissionAudit) *AuditInfo {
	if a == nil {
		return nil
	}
	return &AuditInfo{
		ID:               a.ID,
		OperatorID:       a.OperatorID,
		OperatorUsername: a.OperatorUsername,
		Action:           a.Action,
		Detail:           a.Detail,
		CreatedAt:        a.CreatedAt,
	}
}
