package dto

// PolicyItem 权限策略（p）条目：sub=角色, obj=路径, act=HTTP 方法（正则）。
type PolicyItem struct {
	Sub string `json:"sub"`
	Obj string `json:"obj"`
	Act string `json:"act"`
}

// AddPolicyRequest 新增权限策略请求。
type AddPolicyRequest struct {
	Sub string `json:"sub" binding:"required"`
	Obj string `json:"obj" binding:"required"`
	Act string `json:"act" binding:"required"`
}

// RemovePolicyRequest 删除权限策略请求。
type RemovePolicyRequest struct {
	Sub string `json:"sub" binding:"required"`
	Obj string `json:"obj" binding:"required"`
	Act string `json:"act" binding:"required"`
}

// RoleInheritance 角色继承（g）条目：child 继承 parent。
type RoleInheritance struct {
	Child  string `json:"child"`
	Parent string `json:"parent"`
}

// AddRoleInheritanceRequest 新增角色继承请求。
type AddRoleInheritanceRequest struct {
	Child  string `json:"child" binding:"required"`
	Parent string `json:"parent" binding:"required"`
}

// RemoveRoleInheritanceRequest 删除角色继承请求。
type RemoveRoleInheritanceRequest struct {
	Child  string `json:"child" binding:"required"`
	Parent string `json:"parent" binding:"required"`
}

// AssignRoleRequest 分配用户角色请求。
type AssignRoleRequest struct {
	Roles []string `json:"roles" binding:"required,min=1,dive,oneof=admin author user"`
}
