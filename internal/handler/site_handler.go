package handler

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
	"time"

	"goroutice/internal/config"
	"goroutice/internal/dto"
	"goroutice/internal/service"

	"github.com/gin-gonic/gin"
)

const (
	// feedLimit 是订阅源里输出的文章条数。
	feedLimit = 20
	// sitemapLimit 是站点地图输出的文章条数上限。
	sitemapLimit = 1000
	// siteCacheSeconds 是订阅源与索引文件的缓存时长。
	// 它们的内容只随文章发布变化，而抓取方（阅读器、爬虫）往往是高频轮询，
	// 每次都回源查询数据库纯属浪费。
	siteCacheSeconds = 600
)

// SiteHandler 站点级资源处理器：订阅源与搜索引擎索引文件。
//
// 这些文件刻意不挂在 /api 下：RSS 阅读器和爬虫是按固定路径去根目录找它们的，
// 放在 /api/v1 里等于没人会发现。它们也无需鉴权，内容全部来自已发布文章。
type SiteHandler struct {
	articles *service.ArticleService
	site     config.SiteConfig
}

// NewSiteHandler 构造 SiteHandler。
func NewSiteHandler(articles *service.ArticleService, site config.SiteConfig) *SiteHandler {
	return &SiteHandler{articles: articles, site: site}
}

// rss 是 RSS 2.0 文档结构。
type rss struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	Language    string    `xml:"language,omitempty"`
	LastBuild   string    `xml:"lastBuildDate,omitempty"`
	Items       []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate,omitempty"`
}

// urlSet 是 sitemap 的 urlset 文档结构。
type urlSet struct {
	XMLName xml.Name     `xml:"urlset"`
	Xmlns   string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

type sitemapURL struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}

// RSS 输出站点订阅源（RSS 2.0）。
func (h *SiteHandler) RSS(c *gin.Context) {
	items, _, err := h.articles.ListPublished(1, feedLimit, "", "", "")
	if err != nil {
		handleError(c, err)
		return
	}

	feed := rss{
		Version: "2.0",
		Channel: rssChannel{
			Title:       h.site.Title,
			Link:        h.baseURL(),
			Description: h.site.Description,
			Language:    h.site.Language,
			LastBuild:   time.Now().Format(time.RFC1123Z),
			Items:       make([]rssItem, 0, len(items)),
		},
	}
	for i := range items {
		feed.Channel.Items = append(feed.Channel.Items, h.feedItem(&items[i]))
	}

	writeSiteXML(c, "application/rss+xml; charset=utf-8", feed)
}

// feedItem 把文章摘要转成订阅项。
//
// description 用摘要而不是正文：正文是 Markdown 源文，服务端没有把它渲染成 HTML 的能力，
// 直接塞进订阅源只会让读者看到一堆 `#` 与 `**`；留白反而促使人跳回站点阅读。
// 也正因如此，摘要字段在订阅场景里是必填的，空摘要的条目在阅读器里只有标题。
func (h *SiteHandler) feedItem(a *dto.ArticleSummary) rssItem {
	link := h.articleURL(a.Slug)
	item := rssItem{
		Title:       a.Title,
		Link:        link,
		GUID:        link,
		Description: a.Summary,
	}
	published := a.CreatedAt
	if a.PublishedAt != nil {
		published = *a.PublishedAt
	}
	item.PubDate = published.Format(time.RFC1123Z)
	return item
}

// Sitemap 输出站点地图，供搜索引擎发现文章。
func (h *SiteHandler) Sitemap(c *gin.Context) {
	items, _, err := h.articles.ListPublished(1, sitemapLimit, "", "", "")
	if err != nil {
		handleError(c, err)
		return
	}

	set := urlSet{
		Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9",
		URLs:  make([]sitemapURL, 0, len(items)+1),
	}
	set.URLs = append(set.URLs, sitemapURL{Loc: h.baseURL() + "/"})
	for i := range items {
		set.URLs = append(set.URLs, sitemapURL{
			Loc:     h.articleURL(items[i].Slug),
			LastMod: items[i].UpdatedAt.Format(time.DateOnly),
		})
	}

	writeSiteXML(c, "application/xml; charset=utf-8", set)
}

// Robots 输出 robots.txt。
//
// 只屏蔽 /api/：上传目录 /uploads 里是文章配图，屏蔽它会让图片从搜索结果里消失；
// /admin 是前端路由，不在本服务的路由表里，是否要让爬虫访问由前端站点自己决定。
func (h *SiteHandler) Robots(c *gin.Context) {
	body := fmt.Sprintf("User-agent: *\nAllow: /\nDisallow: /api/\n\nSitemap: %s/sitemap.xml\n", h.baseURL())

	setSiteCache(c)
	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(body))
}

// baseURL 返回去掉结尾斜杠的站点地址。
// 配置里多写一个 `/` 会拼出 `//articles/xxx` 这类地址，部分阅读器会直接判定为非法链接。
func (h *SiteHandler) baseURL() string {
	return strings.TrimRight(h.site.BaseURL, "/")
}

// articleURL 拼接文章详情页地址。路径与前端路由 /articles/:key 保持一致。
func (h *SiteHandler) articleURL(slug string) string {
	return h.baseURL() + "/articles/" + slug
}

// writeSiteXML 序列化并输出 XML，附带缓存头。
//
// 手工拼 xml.Header 而不用 gin 的 c.XML：后者固定把 Content-Type 写成 application/xml，
// 而 RSS 的标准类型是 application/rss+xml，部分阅读器据此判断能否订阅。
func writeSiteXML(c *gin.Context, contentType string, v any) {
	body, err := xml.Marshal(v)
	if err != nil {
		handleError(c, err)
		return
	}
	setSiteCache(c)
	c.Data(http.StatusOK, contentType, append([]byte(xml.Header), body...))
}

// setSiteCache 设置站点级资源的缓存头。
func setSiteCache(c *gin.Context) {
	c.Header("Cache-Control", fmt.Sprintf("public, max-age=%d", siteCacheSeconds))
}
