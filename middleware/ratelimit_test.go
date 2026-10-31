package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestLimiterUsesTheSocketHostAndIgnoresForwardedHeaders(t *testing.T) {
	limiter := NewLimiter(2, time.Minute)
	router := gin.New()
	router.POST("/login", limiter.Handler(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	for i := range 3 {
		req := httptest.NewRequest(http.MethodPost, "/login", nil)
		req.RemoteAddr = "192.0.2.8:" + strconv.Itoa(4000+i)
		req.Header.Set("X-Forwarded-For", "203.0.113."+strconv.Itoa(i))
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		want := http.StatusNoContent
		if i == 2 {
			want = http.StatusTooManyRequests
		}
		if recorder.Code != want {
			t.Fatalf("request %d: want %d, got %d", i, want, recorder.Code)
		}
	}
}
