package model

// 审计动作常量。
const (
	// 权限与角色
	AuditPolicyAdd    = "policy_add"
	AuditPolicyRemove = "policy_remove"
	AuditRoleAdd      = "role_add"
	AuditRoleRemove   = "role_remove"
	AuditAssignRole   = "assign_role"

	// 认证与账户
	AuditRegister       = "register"
	AuditLogin          = "login"
	AuditLoginFailed    = "login_failed"
	AuditLogoutAll      = "logout_all"
	AuditDeleteAccount  = "delete_account"
	AuditChangePassword = "change_password"
	AuditResetPassword  = "reset_password"
	AuditVerifyEmail    = "verify_email"

	// 内容与资源管理
	AuditUserUpdate     = "user_update"
	AuditUserDelete     = "user_delete"
	AuditCategoryCreate = "category_create"
	AuditCategoryUpdate = "category_update"
	AuditCategoryDelete = "category_delete"
	AuditTagCreate      = "tag_create"
	AuditTagUpdate      = "tag_update"
	AuditTagDelete      = "tag_delete"
	AuditArticleCreate  = "article_create"
	AuditArticleUpdate  = "article_update"
	AuditArticleDelete  = "article_delete"
	AuditArticleStatus  = "article_status"
	AuditArticleFeature = "article_feature"
	AuditFileUpload     = "file_upload"
	AuditFileDelete     = "file_delete"
)

// PermissionAudit 权限变更审计记录。
type PermissionAudit struct {
	Base
	OperatorID       string `gorm:"size:36;index" json:"operator_id"`
	OperatorUsername string `gorm:"size:64;index" json:"operator_username"`
	Action           string `gorm:"size:32;index" json:"action"`
	Detail           string `gorm:"size:255" json:"detail"`
}
