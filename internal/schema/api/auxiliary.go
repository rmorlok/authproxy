package api

import (
	"github.com/rmorlok/authproxy/internal/apid"
)

// ErrorResponse is the standardized error response format for authproxy API errors.
//
//	@Description	Standardized error response
type ErrorResponse struct {
	// Error message.
	Error string `json:"error" yaml:"error" example:"Bad Request"`
	// Stack trace, only populated in debug mode.
	StackTrace string `json:"stackTrace,omitempty" yaml:"stackTrace,omitempty"`
}

// SessionInitiateParams is the request body for POST /session/_initiate.
// ReturnToUrl is where the browser should land after host authentication.
type SessionInitiateParams struct {
	ReturnToUrl string `json:"returnToUrl" yaml:"returnToUrl" example:"https://example.com/return"`
}

// SessionInitiateFailureResponse tells the SPA where to redirect when the
// current request cannot establish a session yet.
type SessionInitiateFailureResponse struct {
	RedirectUrl string `json:"redirectUrl" yaml:"redirectUrl" example:"https://example.com/auth"`
}

// SessionInitiateSuccessResponse is returned once a session already exists or
// has been established from the request authentication.
type SessionInitiateSuccessResponse struct {
	ActorId apid.ID `json:"actorId" yaml:"actorId" swaggertype:"string" example:"act_test550e8400abcde"`
}

type KeyValueJson struct {
	Key   string `json:"key" yaml:"key" example:"env"`
	Value string `json:"value" yaml:"value" example:"production"`
}

type PutKeyValueRequestJson struct {
	Value string `json:"value" yaml:"value" example:"production"`
}
