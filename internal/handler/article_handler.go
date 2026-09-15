package handler

import (
	"goroutice/internal/dto"
	"goroutice/internal/middleware"
	"goroutice/internal/model"
	"goroutice/internal/pkg/pagination"
	"goroutice/internal/pkg/response"
	"goroutice/internal/service"

	"github.com/gin-gonic/gin"
)

// ArticleHandler 文章处理器。
type ArticleHandler struct {
	articleService *service.ArticleService
	auditor        *service.AuditService
}

// NewArticleHandler 构造 ArticleHandler。
func NewArticleHandler(articleService *service.ArticleService, auditor *service.AuditService) *ArticleHandler {
	return &ArticleHandler{articleService: articleService, auditor: auditor}
}

// PublicList 公开文章列表。
func (h *ArticleHandler) PublicList(c *gin.Context) {
	pg := pagination.Parse(c.Query("page"), c.Query("size"))
	categoryID, err := parseIDQuery(c, "category_id")
	if err != nil {
		handleError(c, err)
		return
	}
	tagID, err := parseIDQuery(c, "tag_id")
	if err != nil {
		handleError(c, err)
		return
	}

	items, total, err := h.articleService.ListPublished(
		pg.Page, pg.Size,
		c.Query("keyword"),
		categoryID,
		tagID,
	)
	if err != nil {
		handleError(c, err)
		return
	}
	response.Page(c, items, total, pg.Page, pg.Size)
}

// PublicDetail 公开文章详情（按 ID 或 slug）。
func (h *ArticleHandler) PublicDetail(c *gin.Context) {
	item, err := h.articleService.GetPublished(c.Param("key"), viewerKey(c))
	if err != nil {
		handleError(c, err)
		return
	}
	response.Success(c, item)
}

// maxUserAgentLen 限制参与浏览量去重的 User-Agent 长度。
// 该头由客户端任意填充（服务器允许的上限是 MB 级），原样当 map 键会让少量请求就撑起大量内存。
const maxUserAgentLen = 128

// viewerKey 构造浏览量去重键：IP + User-Agent。
// 带上 User-Agent 是为了区分同一出口 IP 下的不同读者（公司内网、运营商 NAT），
// 否则一篇热门文章在同网段里被几十个人读过也只会计一次。
func viewerKey(c *gin.Context) string {
	ua := c.GetHeader("User-Agent")
	if len(ua) > maxUserAgentLen {
		ua = ua[:maxUserAgentLen]
	}
	return c.ClientIP() + "|" + ua
}

// Create 创建文章。
func (h *ArticleHandler) Create(c *gin.Context) {
	var req dto.ArticleRequest
	if !bindJSON(c, &req) {
		return
	}

	item, err := h.articleService.Create(middleware.CurrentUserID(c), req)
	if err != nil {
		handleError(c, err)
		return
	}
	auditLog(c, h.auditor, model.AuditArticleCreate, "article="+item.Slug)
	response.Created(c, item)
}

// Update 更新文章。
func (h *ArticleHandler) Update(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		handleError(c, err)
		return
	}

	var req dto.ArticleRequest
	if !bindJSON(c, &req) {
		return
	}

	item, err := h.articleService.Update(middleware.CurrentUserID(c), middleware.CurrentRoles(c), id, req)
	if err != nil {
		handleError(c, err)
		return
	}
	auditLog(c, h.auditor, model.AuditArticleUpdate, "article="+item.Slug)
	response.Success(c, item)
}

// Delete 删除文章。
func (h *ArticleHandler) Delete(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		handleError(c, err)
		return
	}

	if err := h.articleService.Delete(middleware.CurrentUserID(c), middleware.CurrentRoles(c), id); err != nil {
		handleError(c, err)
		return
	}
	auditLog(c, h.auditor, model.AuditArticleDelete, "id="+id)
	response.Success(c, nil)
}

// MineList 当前作者的文章列表（含草稿）。
func (h *ArticleHandler) MineList(c *gin.Context) {
	pg := pagination.Parse(c.Query("page"), c.Query("size"))
	items, total, err := h.articleService.ListMine(middleware.CurrentUserID(c), pg.Page, pg.Size, c.Query("status"))
	if err != nil {
		handleError(c, err)
		return
	}
	response.Page(c, items, total, pg.Page, pg.Size)
}

// MineDetail 当前作者的文章详情（任意状态）。
func (h *ArticleHandler) MineDetail(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		handleError(c, err)
		return
	}

	item, err := h.articleService.GetMine(middleware.CurrentUserID(c), middleware.CurrentRoles(c), id)
	if err != nil {
		handleError(c, err)
		return
	}
	response.Success(c, item)
}

// AdminList 管理员文章列表（全部状态）。
func (h *ArticleHandler) AdminList(c *gin.Context) {
	pg := pagination.Parse(c.Query("page"), c.Query("size"))
	categoryID, err := parseIDQuery(c, "category_id")
	if err != nil {
		handleError(c, err)
		return
	}

	items, total, err := h.articleService.ListAdmin(
		pg.Page, pg.Size,
		c.Query("keyword"),
		c.Query("status"),
		categoryID,
	)
	if err != nil {
		handleError(c, err)
		return
	}
	response.Page(c, items, total, pg.Page, pg.Size)
}

// UpdateStatus 更新文章状态。
func (h *ArticleHandler) UpdateStatus(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		handleError(c, err)
		return
	}

	var req dto.ArticleStatusRequest
	if !bindJSON(c, &req) {
		return
	}

	if err := h.articleService.UpdateStatus(id, req.Status); err != nil {
		handleError(c, err)
		return
	}
	auditLog(c, h.auditor, model.AuditArticleStatus, "id="+id+" status="+req.Status)
	response.Success(c, nil)
}

// SetFeature 更新文章置顶/推荐标记。
func (h *ArticleHandler) SetFeature(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		handleError(c, err)
		return
	}

	var req dto.ArticleFeatureRequest
	if !bindJSON(c, &req) {
		return
	}

	if err := h.articleService.SetFeature(id, req); err != nil {
		handleError(c, err)
		return
	}
	auditLog(c, h.auditor, model.AuditArticleFeature, "id="+id)
	response.Success(c, nil)
}

// ListRevisions 文章的历史修订列表。
func (h *ArticleHandler) ListRevisions(c *gin.Context) {
	articleID, err := parseID(c)
	if err != nil {
		handleError(c, err)
		return
	}

	pg := pagination.Parse(c.Query("page"), c.Query("size"))
	items, total, err := h.articleService.ListRevisions(
		middleware.CurrentUserID(c), middleware.CurrentRoles(c), articleID, pg.Page, pg.Size,
	)
	if err != nil {
		handleError(c, err)
		return
	}
	response.Page(c, items, total, pg.Page, pg.Size)
}

// GetRevision 历史修订详情（含正文）。
func (h *ArticleHandler) GetRevision(c *gin.Context) {
	articleID, revisionID, ok := parseArticleRevisionIDs(c)
	if !ok {
		return
	}

	item, err := h.articleService.GetRevision(middleware.CurrentUserID(c), middleware.CurrentRoles(c), articleID, revisionID)
	if err != nil {
		handleError(c, err)
		return
	}
	response.Success(c, item)
}

// RestoreRevision 把文章回滚到指定修订。
func (h *ArticleHandler) RestoreRevision(c *gin.Context) {
	articleID, revisionID, ok := parseArticleRevisionIDs(c)
	if !ok {
		return
	}

	item, err := h.articleService.RestoreRevision(middleware.CurrentUserID(c), middleware.CurrentRoles(c), articleID, revisionID)
	if err != nil {
		handleError(c, err)
		return
	}
	auditLog(c, h.auditor, model.AuditArticleRestore, "article="+item.Slug+" revision="+revisionID)
	response.Success(c, item)
}

// parseArticleRevisionIDs 解析修订相关路由上的两个路径参数，失败时已写出响应。
func parseArticleRevisionIDs(c *gin.Context) (articleID, revisionID string, ok bool) {
	articleID, err := parseID(c)
	if err != nil {
		handleError(c, err)
		return "", "", false
	}
	revisionID, err = parseUUIDParam(c, "revisionId")
	if err != nil {
		handleError(c, err)
		return "", "", false
	}
	return articleID, revisionID, true
}
