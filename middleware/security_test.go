package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Harshwagh21/NimbusCloud/middleware"
)

func TestSecurityHeadersAreSetOnEveryResponse(t *testing.T) {
	router := gin.New()
	router.Use(middleware.SecurityHeaders())
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	}
	for header, value := range want {
		if got := recorder.Header().Get(header); got != value {
			t.Errorf("%s: want %q, got %q", header, value, got)
		}
	}
	if recorder.Header().Get("Content-Security-Policy") == "" {
		t.Error("a content security policy must be present")
	}
}
