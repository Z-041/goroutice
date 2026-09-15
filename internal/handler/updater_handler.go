package handler

import (
	"net/http"

	"goroutice/internal/dto"
	"goroutice/internal/pkg/response"
	"goroutice/internal/updater"

	"github.com/gin-gonic/gin"
)

// UpdaterHandler 自动更新状态处理器，仅注册在管理员路由组下。
type UpdaterHandler struct {
	updater *updater.Updater
	version string
}

// NewUpdaterHandler 构造 UpdaterHandler；自动更新未启用时 u 传 nil，version 为当前二进制版本。
func NewUpdaterHandler(u *updater.Updater, version string) *UpdaterHandler {
	return &UpdaterHandler{updater: u, version: version}
}

// Status 返回自动更新状态快照；未启用时仅返回 enabled=false 与当前版本。
func (h *UpdaterHandler) Status(c *gin.Context) {
	response.Success(c, h.snapshot())
}

// Check 立即触发一次版本检查并返回检查后的状态；检查失败的原因记录在 last_error 字段中。
func (h *UpdaterHandler) Check(c *gin.Context) {
	if h.updater == nil {
		response.Error(c, http.StatusConflict, "auto update is disabled")
		return
	}
	// 检查失败不改变响应状态码，失败原因由 last_error 字段承载，便于前端统一渲染提示。
	_ = h.updater.Refresh(c.Request.Context())
	response.Success(c, dto.ToUpdaterStatus(h.updater.Status()))
}

// snapshot 构造状态响应；未启用自动更新时返回降级结构。
func (h *UpdaterHandler) snapshot() *dto.UpdaterStatus {
	if h.updater == nil {
		return &dto.UpdaterStatus{Enabled: false, CurrentVersion: h.version}
	}
	return dto.ToUpdaterStatus(h.updater.Status())
}
