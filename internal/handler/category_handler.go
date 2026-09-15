package handler

import (
	"goroutice/internal/dto"
	"goroutice/internal/model"
	"goroutice/internal/pkg/pagination"
	"goroutice/internal/pkg/response"
	"goroutice/internal/service"

	"github.com/gin-gonic/gin"
)

// CategoryHandler 分类处理器。
type CategoryHandler struct {
	categoryService *service.CategoryService
	auditor         *service.AuditService
}

// NewCategoryHandler 构造 CategoryHandler。
func NewCategoryHandler(categoryService *service.CategoryService, auditor *service.AuditService) *CategoryHandler {
	return &CategoryHandler{categoryService: categoryService, auditor: auditor}
}

// List 分页查询分类。
func (h *CategoryHandler) List(c *gin.Context) {
	pg := pagination.Parse(c.Query("page"), c.Query("size"))
	items, total, err := h.categoryService.List(pg.Page, pg.Size, c.Query("keyword"))
	if err != nil {
		handleError(c, err)
		return
	}
	response.Page(c, items, total, pg.Page, pg.Size)
}

// Get 按 ID 查询分类。
func (h *CategoryHandler) Get(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		handleError(c, err)
		return
	}

	item, err := h.categoryService.GetByID(id)
	if err != nil {
		handleError(c, err)
		return
	}
	response.Success(c, item)
}

// Create 创建分类。
func (h *CategoryHandler) Create(c *gin.Context) {
	var req dto.CategoryRequest
	if !bindJSON(c, &req) {
		return
	}

	item, err := h.categoryService.Create(req)
	if err != nil {
		handleError(c, err)
		return
	}
	auditLog(c, h.auditor, model.AuditCategoryCreate, "category="+item.Slug)
	response.Created(c, item)
}

// Update 更新分类。
func (h *CategoryHandler) Update(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		handleError(c, err)
		return
	}

	var req dto.CategoryRequest
	if !bindJSON(c, &req) {
		return
	}

	item, err := h.categoryService.Update(id, req)
	if err != nil {
		handleError(c, err)
		return
	}
	auditLog(c, h.auditor, model.AuditCategoryUpdate, "category="+item.Slug)
	response.Success(c, item)
}

// Delete 删除分类。
func (h *CategoryHandler) Delete(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		handleError(c, err)
		return
	}

	if err := h.categoryService.Delete(id); err != nil {
		handleError(c, err)
		return
	}
	auditLog(c, h.auditor, model.AuditCategoryDelete, "id="+id)
	response.Success(c, nil)
}
