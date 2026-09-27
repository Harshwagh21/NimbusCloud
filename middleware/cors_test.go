package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Harshwagh21/NimbusCloud/middleware"
)

func corsRouter(origins ...string) *gin.Engine {
	router := gin.New()
	router.Use(middleware.CORS(origins))
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })
	return router
}

func requestFrom(method, origin string) *http.Request {
	request := httptest.NewRequest(method, "/", nil)
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	return request
}

func TestCORSAllowsAConfiguredOrigin(t *testing.T) {
	recorder := httptest.NewRecorder()

	corsRouter("http://localhost:3000").ServeHTTP(recorder, requestFrom(http.MethodGet, "http://localhost:3000"))

	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Errorf("want the origin echoed back, got %q", got)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("refresh cookies require credentialed requests, got %q", got)
	}
}

// Credentialed requests make a wildcard both invalid and dangerous, so an unknown origin
// must simply receive no CORS grant.
func TestCORSRefusesAnUnknownOrigin(t *testing.T) {
	recorder := httptest.NewRecorder()

	corsRouter("http://localhost:3000").ServeHTTP(recorder, requestFrom(http.MethodGet, "https://evil.example"))

	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("an unknown origin must not be granted access, got %q", got)
	}
}

func TestCORSNeverRespondsWithAWildcard(t *testing.T) {
	recorder := httptest.NewRecorder()

	corsRouter("http://localhost:3000", "https://nimbus.example").
		ServeHTTP(recorder, requestFrom(http.MethodGet, "https://nimbus.example"))

	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got == "*" {
		t.Error("a wildcard origin is invalid alongside credentials")
	}
}

func TestCORSAnswersPreflightWithoutReachingTheHandler(t *testing.T) {
	recorder := httptest.NewRecorder()
	router := gin.New()
	router.Use(middleware.CORS([]string{"http://localhost:3000"}))
	router.GET("/", func(c *gin.Context) { t.Error("preflight must not reach the handler") })

	router.ServeHTTP(recorder, requestFrom(http.MethodOptions, "http://localhost:3000"))

	if recorder.Code != http.StatusNoContent {
		t.Errorf("want status 204 for a preflight, got %d", recorder.Code)
	}
	if recorder.Header().Get("Access-Control-Allow-Headers") == "" {
		t.Error("preflight must advertise the allowed headers")
	}
}
