package routes

import schemaapi "github.com/rmorlok/authproxy/internal/schema/api"

type KeyValueJson = schemaapi.KeyValueJson
type PutKeyValueRequestJson = schemaapi.PutKeyValueRequestJson

// ErrorResponse is the standardized error response format for authproxy API errors.
//
//	@Description	Standardized error response
type ErrorResponse struct {
	// Error message
	Error string `json:"error" example:"Bad Request"`
	// Stack trace (only in debug mode)
	StackTrace string `json:"stack_trace,omitempty"`
}
