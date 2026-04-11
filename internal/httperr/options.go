package httperr

import (
	"errors"
	"fmt"
)

// Option configures an Error during construction.
type Option func(*Error)

// WithInternalErr sets the internal error. If err wraps an *Error, extracts
// its status and response message (unless already set by the constructor).
func WithInternalErr(err error) Option {
	return func(e *Error) {
		var he *Error
		if errors.As(err, &he) {
			if err == he {
				// Direct *Error: absorb its fields if constructor hasn't set them
				if e.ResponseMsg == "" {
					e.ResponseMsg = he.ResponseMsg
				}
				e.InternalErr = he.InternalErr
			} else {
				// Wrapped *Error: take its metadata, keep the full chain
				if e.ResponseMsg == "" {
					e.ResponseMsg = he.ResponseMsg
				}
				e.InternalErr = err
			}
		} else {
			e.InternalErr = err
		}
	}
}

// WithResponseMsg sets the public response message.
func WithResponseMsg(msg string) Option {
	return func(e *Error) {
		e.ResponseMsg = msg
	}
}

// WithResponseMsgf sets the public response message using a format string.
func WithResponseMsgf(format string, args ...any) Option {
	return func(e *Error) {
		e.ResponseMsg = fmt.Sprintf(format, args...)
	}
}

// WithPublicErr sets both the response message and internal error to the same
// error, making the error message visible to the caller.
func WithPublicErr(err error) Option {
	return func(e *Error) {
		e.ResponseMsg = err.Error()
		e.InternalErr = err
	}
}

// WithInternalErrf sets the internal error using a format string, equivalent to
// WithInternalErr(fmt.Errorf(format, args...)).
func WithInternalErrorf(format string, args ...any) Option {
	return WithInternalErr(fmt.Errorf(format, args...))
}

// WithWrap sets the internal error by wrapping err with a context message.
func WithWrap(err error, msg string) Option {
	return func(e *Error) {
		var he *Error
		if errors.As(err, &he) {
			if e.ResponseMsg == "" {
				e.ResponseMsg = he.ResponseMsg
			}
		}
		e.InternalErr = fmt.Errorf("%s: %w", msg, err)
	}
}

// WithWrapf sets the internal error by wrapping err with a formatted context message.
func WithWrapf(err error, format string, args ...any) Option {
	return func(e *Error) {
		WithWrap(err, fmt.Sprintf(format, args...))(e)
	}
}
