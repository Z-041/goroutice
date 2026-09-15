package handler

import (
	"context"
	"net/http"
	"time"

	"goroutice/internal/pkg/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// readinessPingTimeout 是就绪探针探测数据库的超时时间。
const readinessPingTimeout = 2 * time.Second

// HealthHandler 健康检查处理器。
type HealthHandler struct {
	db *gorm.DB
}

// NewHealthHandler 构造 HealthHandler。
func NewHealthHandler(db *gorm.DB) *HealthHandler {
	return &HealthHandler{db: db}
}

// Liveness 存活探针，仅表示进程存活。
func (h *HealthHandler) Liveness(c *gin.Context) {
	response.Success(c, gin.H{"status": "up"})
}

// Readiness 就绪探针，检查数据库连通性。
func (h *HealthHandler) Readiness(c *gin.Context) {
	sqlDB, err := h.db.DB()
	if err != nil {
		response.Error(c, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	// 带超时探测：数据库不可达时 Ping 可能长时间阻塞，会让探针本身被拖死并堆积请求。
	ctx, cancel := context.WithTimeout(c.Request.Context(), readinessPingTimeout)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		response.Error(c, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	response.Success(c, gin.H{"status": "ready"})
}
