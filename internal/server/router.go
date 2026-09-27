// Package server assembles the HTTP router from the application's dependencies.
package server

import (
	"github.com/gin-gonic/gin"

	"github.com/Harshwagh21/NimbusCloud/config"
	"github.com/Harshwagh21/NimbusCloud/internal/health"
	"github.com/Harshwagh21/NimbusCloud/internal/platform/apierr"
	"github.com/Harshwagh21/NimbusCloud/internal/platform/httpx"
	"github.com/Harshwagh21/NimbusCloud/middleware"
)

// Dependencies is everything the router needs, injected rather than constructed here, so the
// wiring stays visible in one place and the router is testable in isolation.
type Dependencies struct {
	Config config.Config
	Health *health.Handler
}

// NewRouter builds the engine. Domain routes are registered under /api/v1 from Phase 2 on.
func NewRouter(deps Dependencies) *gin.Engine {
	if deps.Config.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	// No proxy is trusted by default; a forwarded-for header would otherwise let a client
	// choose the IP that rate limiting and audit logs record.
	_ = router.SetTrustedProxies(nil)

	router.Use(
		middleware.RequestID(),
		middleware.Logger(),
		middleware.Recovery(),
		middleware.SecurityHeaders(),
		middleware.CORS(deps.Config.CORSAllowedOrigins),
	)

	registerProbes(router, deps.Health)
	registerFallbacks(router)

	router.Group("/api/v1")

	return router
}

// Probes sit outside /api/v1 because they belong to the platform, not to the product API,
// and their paths should never change with an API version.
func registerProbes(router *gin.Engine, handler *health.Handler) {
	router.GET("/healthz", handler.Live)
	router.GET("/readyz", handler.Ready)
}

// Unmatched routes return the same error envelope as everything else, so a client never has
// to parse two different failure shapes.
func registerFallbacks(router *gin.Engine) {
	router.NoRoute(func(c *gin.Context) {
		httpx.Error(c, apierr.NotFound("ROUTE_NOT_FOUND", "the requested endpoint does not exist"))
	})
	router.NoMethod(func(c *gin.Context) {
		httpx.Error(c, apierr.BadRequest("METHOD_NOT_ALLOWED", "that method is not supported on this endpoint"))
	})
}
