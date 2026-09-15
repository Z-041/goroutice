package handler

import (
	"net/http"

	"goroutice/internal/middleware"
	"goroutice/internal/model"
	"goroutice/internal/pkg/pagination"
	"goroutice/internal/pkg/response"
	"goroutice/internal/service"

	"github.com/gin-gonic/gin"
)

// FileHandler 文件处理器。
type FileHandler struct {
	fileService *service.FileService
	auditor     *service.AuditService
}

// NewFileHandler 构造 FileHandler。
func NewFileHandler(fileService *service.FileService, auditor *service.AuditService) *FileHandler {
	return &FileHandler{fileService: fileService, auditor: auditor}
}

// Upload 上传文件。
func (h *FileHandler) Upload(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		// 分块传输超限时 multipart 解析报的是读取错误，按 413 返回而不是「缺少文件」。
		if httpStatusForBodyError(err) == http.StatusRequestEntityTooLarge {
			response.Error(c, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		response.Error(c, http.StatusBadRequest, "file is required")
		return
	}

	info, err := h.fileService.Upload(middleware.CurrentUserID(c), file)
	if err != nil {
		handleError(c, err)
		return
	}
	auditLog(c, h.auditor, model.AuditFileUpload, "file="+info.Name)
	response.Created(c, info)
}

// List 分页查询文件。
func (h *FileHandler) List(c *gin.Context) {
	pg := pagination.Parse(c.Query("page"), c.Query("size"))
	items, total, err := h.fileService.List(middleware.CurrentUserID(c), middleware.CurrentRoles(c), pg.Page, pg.Size)
	if err != nil {
		handleError(c, err)
		return
	}
	response.Page(c, items, total, pg.Page, pg.Size)
}

// Delete 删除文件。
func (h *FileHandler) Delete(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		handleError(c, err)
		return
	}

	if err := h.fileService.Delete(middleware.CurrentUserID(c), middleware.CurrentRoles(c), id); err != nil {
		handleError(c, err)
		return
	}
	auditLog(c, h.auditor, model.AuditFileDelete, "id="+id)
	response.Success(c, nil)
}
