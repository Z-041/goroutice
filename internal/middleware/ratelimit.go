package middleware

import (
	"context"
	"net/http"
	"sync"
	"time"

	"goroutice/internal/pkg/response"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// visitor 保存单个客户端的令牌桶与最后访问时间。
type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// tier 表示一个限流档位（每秒令牌数与桶容量）。
type tier struct {
	rps   rate.Limit
	burst int
}

// RateLimiter 分级令牌桶限流器，按 用户/IP + 接口 维度分桶计数。
type RateLimiter struct {
	mu          sync.Mutex
	visitors    map[string]*visitor
	defaultTier tier
	tiers       map[string]tier
}

// NewRateLimiter 构造限流器，rps 为默认档每秒放行令牌数，burst 为桶容量。
func NewRateLimiter(rps float64, burst int) *RateLimiter {
	return &RateLimiter{
		visitors:    make(map[string]*visitor),
		defaultTier: tier{rps: rate.Limit(rps), burst: burst},
		tiers:       make(map[string]tier),
	}
}

// SetTier 注册命名限流档位，供 WriteHandler(scope) 使用。
func (rl *RateLimiter) SetTier(scope string, rps float64, burst int) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.tiers[scope] = tier{rps: rate.Limit(rps), burst: burst}
}

// Handler 返回作用于所有请求的限流中间件（默认档，按 用户/IP+接口 分桶）。
func (rl *RateLimiter) Handler() gin.HandlerFunc {
	return rl.limit("")
}

// WriteHandler 返回仅作用于写请求（POST/PUT/PATCH/DELETE）的限流中间件。
func (rl *RateLimiter) WriteHandler(scope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !isWriteMethod(c.Request.Method) {
			c.Next()
			return
		}
		rl.limit(scope)(c)
	}
}

func (rl *RateLimiter) limit(scope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !rl.allow(rl.key(c, scope), scope) {
			response.Error(c, http.StatusTooManyRequests, "too many requests")
			c.Abort()
			return
		}
		c.Next()
	}
}

// key 构造分桶键：档位 + 身份（登录用户 ID，否则 IP）+ 接口路径。
func (rl *RateLimiter) key(c *gin.Context, scope string) string {
	identity := CurrentUserID(c)
	if identity == "" {
		identity = "ip:" + c.ClientIP()
	} else {
		identity = "user:" + identity
	}

	route := c.FullPath()
	if route == "" {
		route = c.Request.Method + " " + c.Request.URL.Path
	}

	return scope + "|" + identity + "|" + route
}

func (rl *RateLimiter) allow(key, scope string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	t := rl.defaultTier
	if scoped, ok := rl.tiers[scope]; ok {
		t = scoped
	}

	v, ok := rl.visitors[key]
	if !ok {
		v = &visitor{limiter: rate.NewLimiter(t.rps, t.burst)}
		rl.visitors[key] = v
	}
	v.lastSeen = time.Now()
	return v.limiter.Allow()
}

func isWriteMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// Cleanup 周期性清理超过 idleTimeout 未访问的客户端记录，避免内存无限增长。
// 当 ctx 取消时退出。
func (rl *RateLimiter) Cleanup(ctx context.Context, idleTimeout time.Duration) {
	ticker := time.NewTicker(idleTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rl.mu.Lock()
			for key, v := range rl.visitors {
				if time.Since(v.lastSeen) > idleTimeout {
					delete(rl.visitors, key)
				}
			}
			rl.mu.Unlock()
		}
	}
}
