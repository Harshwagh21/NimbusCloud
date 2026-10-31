package middleware

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Harshwagh21/NimbusCloud/internal/platform/apierr"
	"github.com/Harshwagh21/NimbusCloud/internal/platform/httpx"
)

type userIDContextKey struct{}

// Authenticate requires a bearer access token and stores the user id on the request.
// parse is injected so this package does not own the signing key.
func Authenticate(parse func(string) (uuid.UUID, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := bearerUser(c.GetHeader("Authorization"), parse)
		if !ok {
			httpx.Error(c, apierr.Unauthorized("UNAUTHENTICATED", "authentication is required"))
			return
		}
		ctx := context.WithValue(c.Request.Context(), userIDContextKey{}, userID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// UserID returns the id Authenticate stored, if this request has one.
func UserID(c *gin.Context) (uuid.UUID, bool) {
	id, ok := c.Request.Context().Value(userIDContextKey{}).(uuid.UUID)
	return id, ok && id != uuid.Nil
}

func bearerUser(header string, parse func(string) (uuid.UUID, error)) (uuid.UUID, bool) {
	raw, ok := bearerToken(header)
	if !ok {
		return uuid.Nil, false
	}
	userID, err := parse(raw)
	if err != nil || userID == uuid.Nil {
		return uuid.Nil, false
	}
	return userID, true
}

func bearerToken(header string) (string, bool) {
	const scheme = "Bearer "
	if len(header) < len(scheme) || !strings.EqualFold(header[:len(scheme)], scheme) {
		return "", false
	}
	token := strings.TrimSpace(header[len(scheme):])
	if token == "" || strings.Contains(token, " ") {
		return "", false
	}
	return token, true
}
