package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestAuthenticateRequiresABearerToken(t *testing.T) {
	userID := uuid.New()
	router := gin.New()
	router.GET("/me", Authenticate(func(token string) (uuid.UUID, error) {
		if token != "good" {
			return uuid.Nil, errBadToken
		}
		return userID, nil
	}), func(c *gin.Context) {
		got, ok := UserID(c)
		if !ok || got != userID {
			c.Status(http.StatusForbidden)
			return
		}
		c.Status(http.StatusNoContent)
	})

	cases := []struct {
		name   string
		header string
		want   int
	}{
		{name: "missing", want: http.StatusUnauthorized},
		{name: "basic", header: "Basic good", want: http.StatusUnauthorized},
		{name: "bad bearer", header: "Bearer nope", want: http.StatusUnauthorized},
		{name: "good bearer", header: "Bearer good", want: http.StatusNoContent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/me", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)
			if recorder.Code != tc.want {
				t.Fatalf("want %d, got %d", tc.want, recorder.Code)
			}
		})
	}
}

type badTokenError struct{}

func (badTokenError) Error() string { return "bad token" }

var errBadToken badTokenError
