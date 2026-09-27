// Package middleware holds the cross-cutting HTTP concerns applied to every request.
package middleware

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const HeaderRequestID = "X-Request-Id"

type contextKey struct{}

var requestIDKey contextKey

// RequestID gives every request an identifier that appears in the response header and in
// every log line, so one id ties a user-reported failure to its server-side trace.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := trustedID(c.GetHeader(HeaderRequestID))

		c.Header(HeaderRequestID, id)
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), requestIDKey, id))
		c.Next()
	}
}

// trustedID reuses an inbound id only when it is a well-formed UUID. The value reaches log
// files, so an arbitrary client-supplied string is not something to propagate.
func trustedID(inbound string) string {
	if _, err := uuid.Parse(inbound); err == nil {
		return inbound
	}
	return uuid.NewString()
}

// RequestIDFrom returns the current request id, or an empty string outside a request.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}
