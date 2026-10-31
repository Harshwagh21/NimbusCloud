// Package auth is the HTTP edge for register, login, refresh, logout, and the
// current user. It binds input, sets the refresh cookie, and writes the envelope.
package auth

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Harshwagh21/NimbusCloud/internal/platform/apierr"
	"github.com/Harshwagh21/NimbusCloud/internal/platform/httpx"
	authsvc "github.com/Harshwagh21/NimbusCloud/internal/service/auth"
	"github.com/Harshwagh21/NimbusCloud/middleware"
)

type authenticator interface {
	Register(ctx context.Context, email, name, password string) (authsvc.Result, error)
	Login(ctx context.Context, email, password string) (authsvc.Result, error)
	Refresh(ctx context.Context, rawToken string) (authsvc.Result, error)
	Logout(ctx context.Context, rawToken string) error
	CurrentUser(ctx context.Context, userID uuid.UUID) (authsvc.Profile, error)
}

// Handler serves the auth routes.
type Handler struct {
	svc        authenticator
	refreshTTL time.Duration
}

func New(svc authenticator, refreshTTL time.Duration) *Handler {
	return &Handler{svc: svc, refreshTTL: refreshTTL}
}

// Mount attaches the auth routes under /api/v1.
func Mount(v1 *gin.RouterGroup, handler *Handler, parse func(string) (uuid.UUID, error), limit gin.HandlerFunc) {
	if limit == nil {
		limit = func(c *gin.Context) { c.Next() }
	}
	group := v1.Group("/auth")
	group.POST("/register", limit, handler.Register)
	group.POST("/login", limit, handler.Login)
	group.POST("/refresh", handler.Refresh)
	group.POST("/logout", handler.Logout)

	authed := v1.Group("")
	authed.Use(middleware.Authenticate(parse))
	authed.GET("/me", handler.Me)
}

func (h *Handler) Register(c *gin.Context) {
	var req registerRequest
	if !bindJSON(c, &req) {
		return
	}
	result, err := h.svc.Register(c.Request.Context(), req.Email, req.Name, req.Password)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	h.writeSession(c, http.StatusCreated, result)
}

func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if !bindJSON(c, &req) {
		return
	}
	result, err := h.svc.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	h.writeSession(c, http.StatusOK, result)
}

func (h *Handler) Refresh(c *gin.Context) {
	raw, err := c.Cookie(refreshCookieName)
	if err != nil || raw == "" {
		httpx.Error(c, apierr.Unauthorized("INVALID_REFRESH_TOKEN", "session is no longer valid"))
		return
	}
	result, err := h.svc.Refresh(c.Request.Context(), raw)
	if err != nil {
		h.clearRefreshCookie(c)
		httpx.Error(c, err)
		return
	}
	h.writeSession(c, http.StatusOK, result)
}

func (h *Handler) Logout(c *gin.Context) {
	raw, _ := c.Cookie(refreshCookieName)
	if err := h.svc.Logout(c.Request.Context(), raw); err != nil {
		httpx.Error(c, err)
		return
	}
	h.clearRefreshCookie(c)
	httpx.NoContent(c)
}

func (h *Handler) Me(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		httpx.Error(c, apierr.Unauthorized("UNAUTHENTICATED", "authentication is required"))
		return
	}
	profile, err := h.svc.CurrentUser(c.Request.Context(), userID)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, newUserResponse(profile))
}

func (h *Handler) writeSession(c *gin.Context, status int, result authsvc.Result) {
	h.setRefreshCookie(c, result.RefreshToken)
	c.JSON(status, newSessionResponse(result))
}

func bindJSON(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		httpx.Error(c, apierr.BadRequest("VALIDATION_ERROR", "request body is invalid"))
		return false
	}
	return true
}
