package auth

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	refreshCookieName = "nimbus_refresh"
	refreshCookiePath = "/api/v1/auth"
)

func (h *Handler) setRefreshCookie(c *gin.Context, raw string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(refreshCookieName, raw, cookieMaxAge(h.refreshTTL), refreshCookiePath, "", true, true)
}

func (h *Handler) clearRefreshCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(refreshCookieName, "", -1, refreshCookiePath, "", true, true)
}

func cookieMaxAge(ttl time.Duration) int {
	seconds := int(ttl.Seconds())
	if seconds < 0 {
		return 0
	}
	return seconds
}
