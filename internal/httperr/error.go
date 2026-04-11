package httperr

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/rmorlok/authproxy/internal/apctx"
)

// Error is an HTTP-aware error with dual tracks: a public response message
// for callers and an internal error for logging/debugging.
type Error struct {
	Status      int
	ResponseMsg string
	InternalErr error
}

func (e *Error) Error() string {
	if e.InternalErr != nil {
		return e.InternalErr.Error()
	}
	if e.ResponseMsg != "" {
		return e.ResponseMsg
	}
	if e.Status != 0 {
		return fmt.Sprintf("HTTP %d: %s", e.Status, StatusText(e.Status))
	}
	return "Unknown error"
}

func (e *Error) Unwrap() error {
	return e.InternalErr
}

// ResponseMsgOrDefault returns the response message, falling back to the
// standard HTTP status text for the error's status code.
func (e *Error) ResponseMsgOrDefault() string {
	if e.ResponseMsg != "" {
		return e.ResponseMsg
	}
	return StatusText(e.Status)
}

// ErrorResponse is the standardized JSON error response format.
type ErrorResponse struct {
	Error      string `json:"error"`
	StackTrace string `json:"stack_trace,omitempty"`
}

// ToErrorResponse converts the error to a JSON-serializable ErrorResponse.
func (e *Error) ToErrorResponse(ctx context.Context) *ErrorResponse {
	resp := &ErrorResponse{
		Error: e.ResponseMsgOrDefault(),
	}
	if apctx.IsDebugMode(ctx) {
		if e.InternalErr != nil {
			resp.StackTrace = fmt.Sprintf("%+v", e.InternalErr)
		}
	}
	return resp
}

// LogError logs the error at the appropriate level based on status code.
func (e *Error) LogError(logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}

	responseMsg := e.ResponseMsgOrDefault()
	if e.Status >= http.StatusInternalServerError {
		logger.Error("api error", "status", e.Status, "response_msg", responseMsg, "error", e.InternalErr)
	} else if e.Status == http.StatusUnauthorized {
		// No logging for unauthorized
	} else if e.Status >= http.StatusBadRequest {
		logger.Warn("api error", "status", e.Status, "response_msg", responseMsg, "error", e.InternalErr)
	}
}

// WriteResponse writes the error as a JSON response to an http.ResponseWriter.
func (e *Error) WriteResponse(ctx context.Context, logger *slog.Logger, w http.ResponseWriter) {
	e.LogError(logger)

	if e.InternalErr != nil {
		AddDebugHeaderError(ctx, w, e.InternalErr)
	}

	errorResponse := e.ToErrorResponse(ctx)
	response, _ := json.Marshal(errorResponse)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.Status)
	w.Write(response)
}

// StatusText returns a human-readable string for common HTTP status codes.
func StatusText(status int) string {
	switch status {
	case 200:
		return "OK"
	case 201:
		return "Created"
	case 400:
		return "Bad Request"
	case 401:
		return "Unauthorized"
	case 403:
		return "Forbidden"
	case 404:
		return "Not Found"
	case 409:
		return "Conflict"
	case 422:
		return "Unprocessable Entity"
	case 500:
		return "Internal Server Error"
	case 502:
		return "Bad Gateway"
	case 503:
		return "Service Unavailable"
	default:
		return "Unknown Status"
	}
}
