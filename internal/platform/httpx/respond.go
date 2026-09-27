// Package httpx holds the shared response helpers every handler uses, so the error envelope
// and status mapping exist in exactly one place.
package httpx

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Harshwagh21/NimbusCloud/internal/platform/apierr"
)

// errorEnvelope is the shape of every failure response:
//
//	{"error": {"code": "FILE_NOT_FOUND", "message": "file not found"}}
//
// Clients correlate a failure with server logs using the X-Request-Id response header, which
// is why the id is not repeated in the body.
type errorEnvelope struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Error writes a failure response and logs anything that is genuinely unexpected.
// Client mistakes are not logged as errors, so a 404 never pollutes the error rate metric.
func Error(c *gin.Context, err error) {
	failure := apierr.From(err)
	status := apierr.Status(failure)

	if status >= http.StatusInternalServerError {
		slog.ErrorContext(c.Request.Context(), "request failed",
			"error", failure.Error(),
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
		)
	}

	c.AbortWithStatusJSON(status, errorEnvelope{Error: errorDetail{
		Code:    failure.Code,
		Message: failure.Message,
	}})
}

// OK writes a successful JSON response.
func OK(c *gin.Context, body any) { c.JSON(http.StatusOK, body) }

// Created writes a 201 for a resource that now exists.
func Created(c *gin.Context, body any) { c.JSON(http.StatusCreated, body) }

// NoContent writes a 204 for a successful operation with nothing to return.
func NoContent(c *gin.Context) { c.Status(http.StatusNoContent) }
