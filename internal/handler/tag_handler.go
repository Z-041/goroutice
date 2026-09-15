package handler

import (
	"goroutice/internal/dto"
	"goroutice/internal/model"
	"goroutice/internal/pkg/pagination"
	"goroutice/internal/pkg/response"
	"goroutice/internal/service"

	"github.com/gin-gonic/gin"
)

// TagHandler 标签处理器。
type TagHandler struct {
	tagService *service.TagService
	auditor    *service.AuditService
}

// NewTagHandler 构造 TagHandler。
func NewTagHandler(tagService *service.TagService, auditor *service.AuditService) *TagHandler {
	return &TagHandler{tagService: tagService, auditor: auditor}
}

// List 分页查询标签。
func (h *TagHandler) List(c *gin.Context) {
	pg := pagination.Parse(c.Query("page"), c.Query("size"))
	items, total, err := h.tagService.List(pg.Page, pg.Size, c.Query("keyword"))
	if err != nil {
		handleError(c, err)
		return
	}
	response.Page(c, items, total, pg.Page, pg.Size)
}

// Get 按 ID 查询标签。
func (h *TagHandler) Get(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		handleError(c, err)
		return
	}

	item, err := h.tagService.GetByID(id)
	if err != nil {
		handleError(c, err)
		return
	}
	response.Success(c, item)
}

// Create 创建标签。
func (h *TagHandler) Create(c *gin.Context) {
	var req dto.TagRequest
	if !bindJSON(c, &req) {
		return
	}

	item, err := h.tagService.Create(req)
	if err != nil {
		handleError(c, err)
		return
	}
	auditLog(c, h.auditor, model.AuditTagCreate, "tag="+item.Slug)
	response.Created(c, item)
}

// Update 更新标签。
func (h *TagHandler) Update(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		handleError(c, err)
		return
	}

	var req dto.TagRequest
	if !bindJSON(c, &req) {
		return
	}

	item, err := h.tagService.Update(id, req)
	if err != nil {
		handleError(c, err)
		return
	}
	auditLog(c, h.auditor, model.AuditTagUpdate, "tag="+item.Slug)
	response.Success(c, item)
}

// Delete 删除标签。
func (h *TagHandler) Delete(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		handleError(c, err)
		return
	}

	if err := h.tagService.Delete(id); err != nil {
		handleError(c, err)
		return
	}
	auditLog(c, h.auditor, model.AuditTagDelete, "id="+id)
	response.Success(c, nil)
}
