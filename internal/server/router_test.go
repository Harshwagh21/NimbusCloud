package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Harshwagh21/NimbusCloud/config"
	"github.com/Harshwagh21/NimbusCloud/internal/health"
	"github.com/Harshwagh21/NimbusCloud/internal/server"
	"github.com/Harshwagh21/NimbusCloud/middleware"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

func testRouter() *gin.Engine {
	return server.NewRouter(server.Dependencies{
		Config: config.Config{AppEnv: config.EnvTest, CORSAllowedOrigins: []string{"http://localhost:3000"}},
		Health: health.New("test"),
	})
}

func get(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	testRouter().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder
}

func TestProbesAreServedOutsideTheVersionedAPI(t *testing.T) {
	for _, path := range []string{"/healthz", "/readyz"} {
		if recorder := get(t, path); recorder.Code != http.StatusOK {
			t.Errorf("%s: want status 200, got %d", path, recorder.Code)
		}
	}
}

// A client should never have to parse two different failure shapes, so even an unmatched
// route returns the standard error envelope.
func TestUnknownRouteReturnsTheStandardErrorEnvelope(t *testing.T) {
	recorder := get(t, "/api/v1/does-not-exist")

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("want status 404, got %d", recorder.Code)
	}

	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("response was not valid JSON: %v (%s)", err, recorder.Body.String())
	}
	if body.Error.Code != "ROUTE_NOT_FOUND" {
		t.Errorf("want code ROUTE_NOT_FOUND, got %q", body.Error.Code)
	}
	if body.Error.Message == "" {
		t.Error("the envelope must carry a human-readable message")
	}
}

func TestEveryResponseCarriesARequestIDAndSecurityHeaders(t *testing.T) {
	recorder := get(t, "/healthz")

	if recorder.Header().Get(middleware.HeaderRequestID) == "" {
		t.Error("responses must carry a request id for log correlation")
	}
	if recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("security headers must apply to probe responses too")
	}
}

// Gin's default recovery writes plain text; the project's own must produce the JSON
// envelope so a panic is indistinguishable from any other 500 to a client.
func TestAPanicBecomesAJSONInternalError(t *testing.T) {
	router := testRouter()
	router.GET("/boom", func(c *gin.Context) { panic("deliberate test panic") })
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/boom", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("want status 500, got %d", recorder.Code)
	}
	var body map[string]map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("a panic response must still be JSON: %v (%s)", err, recorder.Body.String())
	}
	if message := body["error"]["message"]; message == "" {
		t.Error("the envelope must carry a message")
	} else if message == "deliberate test panic" {
		t.Error("the panic value must not be exposed to the client")
	}
}
