package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORS grants access to an explicit allowlist of origins.
//
// Requests carry the refresh cookie, and a wildcard origin is invalid on credentialed
// requests, so the matched origin is echoed back individually and unknown origins simply
// receive no grant.
func CORS(allowedOrigins []string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[origin] = struct{}{}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if _, ok := allowed[origin]; ok {
			grantAccess(c, origin)
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func grantAccess(c *gin.Context, origin string) {
	header := c.Writer.Header()
	header.Set("Access-Control-Allow-Origin", origin)
	header.Set("Access-Control-Allow-Credentials", "true")
	header.Set("Access-Control-Allow-Methods", strings.Join([]string{
		http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete, http.MethodOptions,
	}, ", "))
	header.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, "+HeaderRequestID)
	header.Set("Access-Control-Expose-Headers", HeaderRequestID)
	header.Set("Access-Control-Max-Age", "600")
	// Responses vary by origin, so a shared cache must not reuse one origin's grant.
	header.Add("Vary", "Origin")
}
