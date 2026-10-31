package middleware

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/harshwagh21/reliq/pkg/chain"
	"github.com/harshwagh21/reliq/pkg/ratelimit"

	"github.com/Harshwagh21/NimbusCloud/internal/platform/apierr"
	"github.com/Harshwagh21/NimbusCloud/internal/platform/httpx"
)

// AuthLimit admits register and login through one shared token bucket.
// The count lives in this process and is forgotten on restart. Phase 11 moves it to Redis.
func AuthLimit(limiter *ratelimit.Limiter) gin.HandlerFunc {
	admit := limiter.Policy()
	return func(c *gin.Context) {
		err := chain.Do(c.Request.Context(), func(context.Context) error {
			c.Next()
			return nil
		}, admit)
		if errors.Is(err, chain.ErrRateLimited) {
			httpx.Error(c, apierr.RateLimited("RATE_LIMITED", "too many attempts, try again shortly"))
		}
	}
}
