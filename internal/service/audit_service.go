package service

import (
	"log/slog"

	"goroutice/internal/dto"
	"goroutice/internal/model"
	"goroutice/internal/repository"
)

// Operator 审计操作者信息。
type Operator struct {
	ID       string
	Username string
}

// auditDetailMaxLen 是审计详情列宽，与 model.PermissionAudit 的 size:255 一致。
const auditDetailMaxLen = 255

// AuditService 负责写入与查询审计日志。
type AuditService struct {
	repo *repository.PermissionAuditRepository
}

// NewAuditService 构造 AuditService。
func NewAuditService(repo *repository.PermissionAuditRepository) *AuditService {
	return &AuditService{repo: repo}
}

// Record 写入一条审计记录。
// 审计属旁路能力：写失败仅记录日志，不阻断主流程；接收者为 nil 时静默跳过。
func (s *AuditService) Record(operator Operator, action, detail string) {
	if s == nil || s.repo == nil {
		return
	}
	entry := &model.PermissionAudit{
		OperatorID:       operator.ID,
		OperatorUsername: operator.Username,
		Action:           action,
		// 详情常拼接用户可控内容（文件名等），超长会在写库时报错并被静默丢弃，
		// 反而丢失整条审计记录；这里按列宽截断，保证记录本身落库。
		Detail: truncateRunes(detail, auditDetailMaxLen),
	}
	if err := s.repo.Create(entry); err != nil {
		slog.Error("write audit log", "action", action, "err", err)
	}
}

// List 分页查询审计记录，可按操作者/动作/详情模糊过滤。
func (s *AuditService) List(page, size int, keyword string) ([]dto.AuditInfo, int64, error) {
	if s == nil || s.repo == nil {
		return []dto.AuditInfo{}, 0, nil
	}
	entries, total, err := s.repo.List(page, size, keyword)
	if err != nil {
		return nil, 0, err
	}

	infos := make([]dto.AuditInfo, 0, len(entries))
	for i := range entries {
		infos = append(infos, *dto.ToAuditInfo(&entries[i]))
	}
	return infos, total, nil
}
