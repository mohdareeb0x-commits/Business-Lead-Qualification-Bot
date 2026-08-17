// Package apierr exposes a small, consistent API-error format used by every
// HTTP handler. The shape mirrors the spec:
//
//	{ "error": { "code": "...", "message": "...", "details": { ... } } }
package apierr

import "github.com/gin-gonic/gin"

// Code is a machine-readable error category.
type Code string

const (
	CodeValidation   Code = "VALIDATION_ERROR"
	CodeNotFound     Code = "NOT_FOUND"
	CodeConflict     Code = "CONFLICT"
	CodeUnauthorized Code = "UNAUTHORIZED"
	CodeInternal     Code = "INTERNAL_ERROR"
)

// ErrorBody is the wire shape of an error.
type ErrorBody struct {
	Code    Code        `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

// Envelope is the full response wrapping a single ErrorBody.
type Envelope struct {
	Error ErrorBody `json:"error"`
}

// Write serializes a consistent error response and aborts the request.
func Write(c *gin.Context, status int, code Code, message string, details interface{}) {
	c.AbortWithStatusJSON(status, Envelope{Error: ErrorBody{
		Code:    code,
		Message: message,
		Details: details,
	}})
}
