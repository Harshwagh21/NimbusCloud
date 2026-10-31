package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/harshwagh21/reliq/pkg/ratelimit"
)

func TestAuthLimitSharesOneBucketAcrossClients(t *testing.T) {
	limiter, err := ratelimit.New(1, 2)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.POST("/login", AuthLimit(limiter), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	for i, addr := range []string{"192.0.2.8:4000", "198.51.100.2:4001", "203.0.113.4:4002"} {
		recorder := postLogin(router, addr, "203.0.113."+strconv.Itoa(i))
		want := http.StatusNoContent
		if i == 2 {
			want = http.StatusTooManyRequests
		}
		if recorder.Code != want {
			t.Fatalf("request %d from %s: want %d, got %d body %s", i, addr, want, recorder.Code, recorder.Body.String())
		}
		if i == 2 && !strings.Contains(recorder.Body.String(), `"code":"RATE_LIMITED"`) {
			t.Fatalf("body: %s", recorder.Body.String())
		}
	}
}

func postLogin(router *gin.Engine, remote, forwarded string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.RemoteAddr = remote
	req.Header.Set("X-Forwarded-For", forwarded)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}
