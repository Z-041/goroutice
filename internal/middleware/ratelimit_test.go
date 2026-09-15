package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRateLimiter_BlocksAfterBurst(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rl := NewRateLimiter(1, 1)
	r := gin.New()
	r.Use(rl.Handler())
	r.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	// 第一个令牌放行。
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, httptest.NewRequest(http.MethodGet, "/", nil))
	if w1.Code != http.StatusOK {
		t.Fatalf("first request status = %d, want 200", w1.Code)
	}

	// 令牌耗尽后应被限流。
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/", nil))
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request status = %d, want 429", w2.Code)
	}
}

func TestRateLimiter_WriteHandlerSkipsReads(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rl := NewRateLimiter(1, 1)
	rl.SetTier("write", 1, 1)
	r := gin.New()
	r.Use(rl.WriteHandler("write"))
	r.GET("/get", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	r.POST("/post", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	// GET 请求不受写接口限流影响。
	for i := range 3 {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/get", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("GET #%d status = %d, want 200", i+1, w.Code)
		}
	}

	// POST 请求按写接口档位限流：第一个放行，第二个被限流。
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, httptest.NewRequest(http.MethodPost, "/post", nil))
	if w1.Code != http.StatusOK {
		t.Fatalf("first POST status = %d, want 200", w1.Code)
	}
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodPost, "/post", nil))
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("second POST status = %d, want 429", w2.Code)
	}
}
