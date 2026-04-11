package httperr

import (
	"errors"
	"fmt"
	"net/http"
)

func apply(e *Error, opts []Option) *Error {
	for _, o := range opts {
		o(e)
	}
	return e
}

// BadRequest creates a 400 error with the given response message.
func BadRequest(msg string, opts ...Option) *Error {
	return apply(&Error{Status: http.StatusBadRequest, ResponseMsg: msg}, opts)
}

// BadRequestf creates a 400 error with a formatted response message.
func BadRequestf(format string, args ...any) *Error {
	return &Error{Status: http.StatusBadRequest, ResponseMsg: fmt.Sprintf(format, args...)}
}

// BadRequestErr creates a 400 error with an internal error but no public message
// (defaults to "Bad Request").
func BadRequestErr(err error, opts ...Option) *Error {
	return apply(&Error{Status: http.StatusBadRequest, InternalErr: err}, opts)
}

// Unauthorized creates a 401 error with no message (defaults to "Unauthorized").
func Unauthorized(opts ...Option) *Error {
	return apply(&Error{Status: http.StatusUnauthorized}, opts)
}

// UnauthorizedMsg creates a 401 error with a response message.
func UnauthorizedMsg(msg string, opts ...Option) *Error {
	return apply(&Error{Status: http.StatusUnauthorized, ResponseMsg: msg}, opts)
}

// Forbidden creates a 403 error with the given response message.
func Forbidden(msg string, opts ...Option) *Error {
	return apply(&Error{Status: http.StatusForbidden, ResponseMsg: msg}, opts)
}

// Forbiddenf creates a 403 error with a formatted response message.
func Forbiddenf(format string, args ...any) *Error {
	return &Error{Status: http.StatusForbidden, ResponseMsg: fmt.Sprintf(format, args...)}
}

// NotFound creates a 404 error with the given response message.
func NotFound(msg string, opts ...Option) *Error {
	return apply(&Error{Status: http.StatusNotFound, ResponseMsg: msg}, opts)
}

// NotFoundf creates a 404 error with a formatted response message.
func NotFoundf(format string, args ...any) *Error {
	return &Error{Status: http.StatusNotFound, ResponseMsg: fmt.Sprintf(format, args...)}
}

// Conflict creates a 409 error with the given response message.
func Conflict(msg string, opts ...Option) *Error {
	return apply(&Error{Status: http.StatusConflict, ResponseMsg: msg}, opts)
}

// Conflictf creates a 409 error with a formatted response message.
func Conflictf(format string, args ...any) *Error {
	return &Error{Status: http.StatusConflict, ResponseMsg: fmt.Sprintf(format, args...)}
}

// InternalServerError creates a 500 error with no public message.
func InternalServerError(opts ...Option) *Error {
	return apply(&Error{Status: http.StatusInternalServerError}, opts)
}

// InternalServerErrorMsg creates a 500 error with a response message.
func InternalServerErrorMsg(msg string, opts ...Option) *Error {
	return apply(&Error{Status: http.StatusInternalServerError, ResponseMsg: msg}, opts)
}

// InternalServerErrorf creates a 500 error with a formatted response message.
func InternalServerErrorf(format string, args ...any) *Error {
	return &Error{Status: http.StatusInternalServerError, ResponseMsg: fmt.Sprintf(format, args...)}
}

// New creates an error with the given status code and response message.
func New(status int, msg string, opts ...Option) *Error {
	return apply(&Error{Status: status, ResponseMsg: msg}, opts)
}

// Newf creates an error with the given status code and formatted response message.
func Newf(status int, format string, args ...any) *Error {
	return &Error{Status: status, ResponseMsg: fmt.Sprintf(format, args...)}
}

// NewWithErr creates an error with the given status code and internal error.
func NewWithErr(status int, err error, opts ...Option) *Error {
	return apply(&Error{Status: status, InternalErr: err}, opts)
}

// FromErrorf wraps err with a formatted message and extracts an *Error if one
// is wrapped inside it, equivalent to FromError(fmt.Errorf(format, args...)).
func FromErrorf(format string, args ...any) *Error {
	return FromError(fmt.Errorf(format, args...))
}

// FromError extracts an *Error from err if one is wrapped inside it.
// If err is not an *Error, wraps it as a 500. Options can override fields.
func FromError(err error, opts ...Option) *Error {
	var he *Error
	if errors.As(err, &he) {
		// Clone to avoid mutating the original
		clone := &Error{
			Status:      he.Status,
			ResponseMsg: he.ResponseMsg,
			InternalErr: he.InternalErr,
		}
		// If the original error wraps the *Error, keep the full chain
		if err != he {
			clone.InternalErr = err
		}
		return apply(clone, opts)
	}
	return apply(&Error{Status: http.StatusInternalServerError, InternalErr: err}, opts)
}
