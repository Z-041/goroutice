package handler

import (
	"goroutice/internal/dto"
	"goroutice/internal/model"
	"goroutice/internal/pkg/pagination"
	"goroutice/internal/pkg/response"
	"goroutice/internal/service"

	"github.com/gin-gonic/gin"
)

// UserHandler 用户管理处理器（管理员）。
type UserHandler struct {
	userService *service.UserService
	auditor     *service.AuditService
}

// NewUserHandler 构造 UserHandler。
func NewUserHandler(userService *service.UserService, auditor *service.AuditService) *UserHandler {
	return &UserHandler{userService: userService, auditor: auditor}
}

// List 分页查询用户。
func (h *UserHandler) List(c *gin.Context) {
	pg := pagination.Parse(c.Query("page"), c.Query("size"))
	items, total, err := h.userService.List(pg.Page, pg.Size, c.Query("keyword"))
	if err != nil {
		handleError(c, err)
		return
	}
	response.Page(c, items, total, pg.Page, pg.Size)
}

// Update 更新用户。
func (h *UserHandler) Update(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		handleError(c, err)
		return
	}

	var req dto.UpdateUserRequest
	if !bindJSON(c, &req) {
		return
	}

	user, err := h.userService.Update(id, req)
	if err != nil {
		handleError(c, err)
		return
	}
	auditLog(c, h.auditor, model.AuditUserUpdate, "user="+user.Username)
	response.Success(c, user)
}

// Delete 删除用户。
func (h *UserHandler) Delete(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		handleError(c, err)
		return
	}

	if err := h.userService.Delete(id); err != nil {
		handleError(c, err)
		return
	}
	auditLog(c, h.auditor, model.AuditUserDelete, "id="+id)
	response.Success(c, nil)
}
