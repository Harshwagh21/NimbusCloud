package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Harshwagh21/NimbusCloud/middleware"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

func routerWithRequestID() (*gin.Engine, *string) {
	seen := new(string)
	router := gin.New()
	router.Use(middleware.RequestID())
	router.GET("/", func(c *gin.Context) {
		*seen = middleware.RequestIDFrom(c.Request.Context())
		c.Status(http.StatusOK)
	})
	return router, seen
}

func TestRequestIDGeneratesAnIDWhenTheClientSendsNone(t *testing.T) {
	router, seen := routerWithRequestID()
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	header := recorder.Header().Get(middleware.HeaderRequestID)
	if header == "" {
		t.Fatal("response must carry a request id header")
	}
	if _, err := uuid.Parse(header); err != nil {
		t.Errorf("generated id should be a uuid, got %q", header)
	}
	if *seen != header {
		t.Errorf("handler saw %q but the response returned %q", *seen, header)
	}
}

// Propagating an inbound id lets a single trace span the frontend and the API.
func TestRequestIDReusesAValidInboundID(t *testing.T) {
	router, seen := routerWithRequestID()
	inbound := uuid.NewString()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(middleware.HeaderRequestID, inbound)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if *seen != inbound {
		t.Errorf("want the inbound id %q to be reused, got %q", inbound, *seen)
	}
}

// A client-supplied id ends up in logs, so an unbounded or malformed value is rejected
// rather than trusted.
func TestRequestIDReplacesAnUntrustworthyInboundID(t *testing.T) {
	cases := map[string]string{
		"not a uuid":       "../../etc/passwd",
		"excessively long": string(make([]byte, 512)),
		"empty":            "",
	}

	for name, inbound := range cases {
		t.Run(name, func(t *testing.T) {
			router, seen := routerWithRequestID()
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.Header.Set(middleware.HeaderRequestID, inbound)

			router.ServeHTTP(httptest.NewRecorder(), request)

			if *seen == inbound {
				t.Errorf("untrusted id %q should have been replaced", inbound)
			}
			if _, err := uuid.Parse(*seen); err != nil {
				t.Errorf("replacement should be a uuid, got %q", *seen)
			}
		})
	}
}

func TestRequestIDFromReturnsEmptyStringWhenMiddlewareIsAbsent(t *testing.T) {
	if got := middleware.RequestIDFrom(httptest.NewRequest(http.MethodGet, "/", nil).Context()); got != "" {
		t.Errorf("want an empty string outside the middleware, got %q", got)
	}
}
