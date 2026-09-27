package middleware

import (
	"fmt"
	"log/slog"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"github.com/Harshwagh21/NimbusCloud/internal/platform/apierr"
	"github.com/Harshwagh21/NimbusCloud/internal/platform/httpx"
)

// Recovery turns a panic into a logged 500 rather than a dropped connection, so one bad
// request cannot take the process down and the stack trace still reaches the logs.
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}

			slog.ErrorContext(c.Request.Context(), "panic recovered",
				"panic", fmt.Sprint(recovered),
				"method", c.Request.Method,
				"path", c.Request.URL.Path,
				"request_id", RequestIDFrom(c.Request.Context()),
				"stack", string(debug.Stack()),
			)

			httpx.Error(c, apierr.Internal(fmt.Errorf("panic: %v", recovered)))
		}()

		c.Next()
	}
}
