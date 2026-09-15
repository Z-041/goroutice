package handler

import (
	"goroutice/internal/dto"
	"goroutice/internal/pkg/pagination"
	"goroutice/internal/pkg/response"
	"goroutice/internal/service"

	"github.com/gin-gonic/gin"
)

// PolicyHandler 权限策略管理处理器（管理员）。
type PolicyHandler struct {
	policyService *service.PolicyService
}

// NewPolicyHandler 构造 PolicyHandler。
func NewPolicyHandler(policyService *service.PolicyService) *PolicyHandler {
	return &PolicyHandler{policyService: policyService}
}

// List 查询权限策略与角色继承。
func (h *PolicyHandler) List(c *gin.Context) {
	policies, roles, err := h.policyService.ListPolicies()
	if err != nil {
		handleError(c, err)
		return
	}
	response.Success(c, gin.H{
		"policies": policies,
		"roles":    roles,
	})
}

// AddPolicy 新增权限策略。
func (h *PolicyHandler) AddPolicy(c *gin.Context) {
	var req dto.AddPolicyRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := h.policyService.AddPolicy(currentOperator(c), req.Sub, req.Obj, req.Act); err != nil {
		handleError(c, err)
		return
	}
	response.Success(c, nil)
}

// RemovePolicy 删除权限策略。
func (h *PolicyHandler) RemovePolicy(c *gin.Context) {
	var req dto.RemovePolicyRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := h.policyService.RemovePolicy(currentOperator(c), req.Sub, req.Obj, req.Act); err != nil {
		handleError(c, err)
		return
	}
	response.Success(c, nil)
}

// AddRole 新增角色继承。
func (h *PolicyHandler) AddRole(c *gin.Context) {
	var req dto.AddRoleInheritanceRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := h.policyService.AddRoleInheritance(currentOperator(c), req.Child, req.Parent); err != nil {
		handleError(c, err)
		return
	}
	response.Success(c, nil)
}

// RemoveRole 删除角色继承。
func (h *PolicyHandler) RemoveRole(c *gin.Context) {
	var req dto.RemoveRoleInheritanceRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := h.policyService.RemoveRoleInheritance(currentOperator(c), req.Child, req.Parent); err != nil {
		handleError(c, err)
		return
	}
	response.Success(c, nil)
}

// AssignRole 分配用户角色。
func (h *PolicyHandler) AssignRole(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		handleError(c, err)
		return
	}
	var req dto.AssignRoleRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := h.policyService.AssignRole(currentOperator(c), id, req.Roles); err != nil {
		handleError(c, err)
		return
	}
	response.Success(c, nil)
}

// ListAudits 分页查询权限审计记录。
func (h *PolicyHandler) ListAudits(c *gin.Context) {
	pg := pagination.Parse(c.Query("page"), c.Query("size"))
	items, total, err := h.policyService.ListAudits(pg.Page, pg.Size, c.Query("keyword"))
	if err != nil {
		handleError(c, err)
		return
	}
	response.Page(c, items, total, pg.Page, pg.Size)
}
