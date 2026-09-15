package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequestID_GeneratesAndReuses(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(RequestID())
	r.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, CurrentRequestID(c))
	})

	// 未传入时生成新 ID，并写回响应头。
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	id := w.Body.String()
	if id == "" {
		t.Fatal("expected generated request id, got empty")
	}
	if got := w.Header().Get(headerRequestID); got != id {
		t.Fatalf("response header = %q, want %q", got, id)
	}

	// 传入时复用 X-Request-Id。
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set(headerRequestID, "abc-123")
	r.ServeHTTP(w2, req2)
	if got := w2.Body.String(); got != "abc-123" {
		t.Fatalf("expected reused id %q, got %q", "abc-123", got)
	}
}

func TestCurrentRequestID_EmptyWithoutMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, CurrentRequestID(c))
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if got := w.Body.String(); got != "" {
		t.Fatalf("expected empty request id, got %q", got)
	}
}
