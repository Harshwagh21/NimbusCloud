package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Harshwagh21/NimbusCloud/internal/platform/apierr"
	"github.com/Harshwagh21/NimbusCloud/internal/platform/httpx"
)

// ponytail: single-process fixed window, lost on restart. Phase 11 replaces this with Redis.
type Limiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	hits   map[string]windowCount
}

type windowCount struct {
	start time.Time
	count int
}

func NewLimiter(limit int, window time.Duration) *Limiter {
	return &Limiter{limit: limit, window: window, hits: map[string]windowCount{}}
}

func (l *Limiter) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if l.allow(clientIP(c.Request), time.Now()) {
			c.Next()
			return
		}
		httpx.Error(c, apierr.RateLimited("RATE_LIMITED", "too many attempts, try again shortly"))
	}
}

func (l *Limiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.hits) > 4096 {
		l.dropExpired(now)
	}
	slot, ok := l.hits[key]
	if !ok || now.Sub(slot.start) >= l.window {
		l.hits[key] = windowCount{start: now, count: 1}
		return true
	}
	if slot.count >= l.limit {
		return false
	}
	slot.count++
	l.hits[key] = slot
	return true
}

func (l *Limiter) dropExpired(now time.Time) {
	for key, slot := range l.hits {
		if now.Sub(slot.start) >= l.window {
			delete(l.hits, key)
		}
	}
}

// clientIP uses the socket address. Forwarded headers are ignored because the
// router trusts no proxies, and trusting them would let a client pick its limit bucket.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
