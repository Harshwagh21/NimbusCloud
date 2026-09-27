package middleware

import "github.com/gin-gonic/gin"

// SecurityHeaders sets the defensive headers that apply to a JSON API.
//
// The API returns JSON and redirects, never HTML, so the policy is deliberately strict:
// nothing should ever be framed, sniffed, or rendered as a document.
func SecurityHeaders() gin.HandlerFunc {
	headers := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
		// A JSON API needs no sources of its own; this blocks anything a reflected
		// payload might otherwise try to load.
		"Content-Security-Policy":      "default-src 'none'; frame-ancestors 'none'",
		"Cross-Origin-Resource-Policy": "same-site",
	}

	return func(c *gin.Context) {
		for name, value := range headers {
			c.Header(name, value)
		}
		c.Next()
	}
}
