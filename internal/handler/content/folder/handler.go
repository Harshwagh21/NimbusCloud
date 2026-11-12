// Package folder is the HTTP edge for folder CRUD and breadcrumbs.
package folder

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	dbgen "github.com/Harshwagh21/NimbusCloud/db/generated"
	"github.com/Harshwagh21/NimbusCloud/internal/platform/apierr"
	"github.com/Harshwagh21/NimbusCloud/internal/platform/httpx"
	foldersvc "github.com/Harshwagh21/NimbusCloud/internal/service/content/folder"
	"github.com/Harshwagh21/NimbusCloud/middleware"
)

type folderService interface {
	Create(ctx context.Context, ownerID uuid.UUID, name string, parentID *uuid.UUID) (dbgen.Folder, error)
	Get(ctx context.Context, ownerID, id uuid.UUID) (dbgen.Folder, error)
	List(ctx context.Context, ownerID uuid.UUID, parentID *uuid.UUID) ([]dbgen.Folder, error)
	Path(ctx context.Context, ownerID, id uuid.UUID) ([]dbgen.Folder, error)
	Update(ctx context.Context, ownerID, id uuid.UUID, input foldersvc.UpdateInput) (dbgen.Folder, error)
	Delete(ctx context.Context, ownerID, id uuid.UUID) error
}

// Handler serves folder routes.
type Handler struct {
	svc folderService
}

func New(svc folderService) *Handler {
	return &Handler{svc: svc}
}

// Mount attaches folder routes under /api/v1. All routes require a bearer token.
func Mount(v1 *gin.RouterGroup, handler *Handler, parse func(string) (uuid.UUID, error)) {
	authed := v1.Group("")
	authed.Use(middleware.Authenticate(parse))
	authed.POST("/folders", handler.Create)
	authed.GET("/folders", handler.List)
	authed.GET("/folders/:id", handler.Get)
	authed.GET("/folders/:id/path", handler.Path)
	authed.PATCH("/folders/:id", handler.Update)
	authed.DELETE("/folders/:id", handler.Delete)
}

func (h *Handler) Create(c *gin.Context) {
	ownerID, ok := requireUser(c)
	if !ok {
		return
	}
	var req createRequest
	if !bindJSON(c, &req) {
		return
	}
	folder, err := h.svc.Create(c.Request.Context(), ownerID, req.Name, req.ParentID)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.Created(c, newFolderResponse(folder))
}

func (h *Handler) List(c *gin.Context) {
	ownerID, ok := requireUser(c)
	if !ok {
		return
	}
	parentID, err := optionalParentQuery(c)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	folders, err := h.svc.List(c.Request.Context(), ownerID, parentID)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, newFoldersResponse(folders))
}

func (h *Handler) Get(c *gin.Context) {
	ownerID, id, ok := requireUserAndID(c)
	if !ok {
		return
	}
	folder, err := h.svc.Get(c.Request.Context(), ownerID, id)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, newFolderResponse(folder))
}

func (h *Handler) Path(c *gin.Context) {
	ownerID, id, ok := requireUserAndID(c)
	if !ok {
		return
	}
	path, err := h.svc.Path(c.Request.Context(), ownerID, id)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, newPathResponse(path))
}

func (h *Handler) Update(c *gin.Context) {
	ownerID, id, ok := requireUserAndID(c)
	if !ok {
		return
	}
	var req updateRequest
	if !bindJSON(c, &req) {
		return
	}
	folder, err := h.svc.Update(c.Request.Context(), ownerID, id, foldersvc.UpdateInput{
		Name:      req.Name,
		ParentID:  req.ParentID,
		ParentSet: req.ParentSet,
	})
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, newFolderResponse(folder))
}

func (h *Handler) Delete(c *gin.Context) {
	ownerID, id, ok := requireUserAndID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), ownerID, id); err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.NoContent(c)
}

func requireUser(c *gin.Context) (uuid.UUID, bool) {
	userID, ok := middleware.UserID(c)
	if !ok {
		httpx.Error(c, apierr.Unauthorized("UNAUTHENTICATED", "authentication is required"))
		return uuid.Nil, false
	}
	return userID, true
}

func requireUserAndID(c *gin.Context) (uuid.UUID, uuid.UUID, bool) {
	userID, ok := requireUser(c)
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, apierr.BadRequest("VALIDATION_ERROR", "folder id is invalid"))
		return uuid.Nil, uuid.Nil, false
	}
	return userID, id, true
}

func optionalParentQuery(c *gin.Context) (*uuid.UUID, error) {
	raw := c.Query("parent_id")
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, apierr.BadRequest("VALIDATION_ERROR", "parent_id is invalid")
	}
	return &id, nil
}

func bindJSON(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		httpx.Error(c, apierr.BadRequest("VALIDATION_ERROR", "request body is invalid"))
		return false
	}
	return true
}
