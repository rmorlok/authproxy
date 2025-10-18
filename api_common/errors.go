package api_common

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

// HttpStatusError is an error that allows inner code to drive final HTTP errors. Has two tracks for error messages:
// internal for error information that should be constrained to logs, etc and response which is what can be returned
// to the caller.
type HttpStatusError struct {
	Status      int
	ResponseMsg string
	InternalErr error
}

func (e *HttpStatusError) Error() string {
	if e.InternalErr != nil {
		return e.InternalErr.Error()
	}
	if e.ResponseMsg != "" {
		return e.ResponseMsg
	}

	if e.Status != 0 {
		return fmt.Sprintf("HTTP %d: %s", e.Status, httpStatusText(e.Status))
	}

	return "Unknown error"
}

func (e *HttpStatusError) ResponseMsgOrDefault() string {
	if e.ResponseMsg != "" {
		return e.ResponseMsg
	}

	return httpStatusText(e.Status)
}

// ErrorResponse is the standardized error response format for authproxy as it gets serialized to JSON. This normally
// shouldn't be constructed directly but rather constructed using the provided builder. This struct can be used to
// parse errors returned from the authproxy API.
type ErrorResponse struct {
	Error      string `json:"error"`
	StackTrace string `json:"stack_trace,omitempty"`
}

func (e *HttpStatusError) toErrorResponse(cfg Debuggable) *ErrorResponse {
	resp := &ErrorResponse{
		Error: e.ResponseMsgOrDefault(),
	}

	if cfg.IsDebugMode() {
		if e.InternalErr != nil {
			resp.StackTrace = fmt.Sprintf("%+v", e.InternalErr)
		}
	}

	return resp
}

func (e *HttpStatusError) WriteGinResponse(cfg Debuggable, gctx *gin.Context) {
	if e.InternalErr != nil {
		AddGinDebugHeaderError(cfg, gctx, e.InternalErr)
	}

	errorResponse := e.toErrorResponse(cfg)
	gctx.Header("Content-Type", "application/json")
	gctx.PureJSON(e.Status, errorResponse)
}

func (e *HttpStatusError) WriteResponse(cfg Debuggable, w http.ResponseWriter) {
	if e.InternalErr != nil {
		AddDebugHeaderError(cfg, w, e.InternalErr)
	}

	errorResponse := e.toErrorResponse(cfg)

	response, _ := json.Marshal(errorResponse)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.Status)
	w.Write(response)
}

// AsHttpStatusError converts an HTTP status error. If the error is an HTTP status error, it is returned. If an HTTP
// status error is wrapped in the passed error, the status etc will be taken from the wrapped error. This is equivalent
// to using the builder to start from an internal error.
func AsHttpStatusError(err error) *HttpStatusError {
	return NewHttpStatusErrorBuilder().
		WithInternalErr(err).
		BuildStatusError()
}

func httpStatusText(status int) string {
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

type HttpStatusErrorBuilder interface {
	// WithStatus sets the http status of the error to a specific value
	WithStatus(status int) HttpStatusErrorBuilder

	WithStatusNotFound() HttpStatusErrorBuilder
	WithStatusBadRequest() HttpStatusErrorBuilder
	WithStatusUnauthorized() HttpStatusErrorBuilder
	WithStatusForbidden() HttpStatusErrorBuilder
	WithStatusInternalServerError() HttpStatusErrorBuilder

	// DefaultStatus sets the http status of error if it has not already been set to a value other than 500
	DefaultStatus(status int) HttpStatusErrorBuilder

	DefaultStatusNotFound() HttpStatusErrorBuilder
	DefaultStatusBadRequest() HttpStatusErrorBuilder
	DefaultStatusUnauthorized() HttpStatusErrorBuilder
	DefaultStatusForbidden() HttpStatusErrorBuilder

	WithResponseMsg(msg string) HttpStatusErrorBuilder
	WithResponseMsgf(format string, args ...interface{}) HttpStatusErrorBuilder
	DefaultResponseMsg(msg string) HttpStatusErrorBuilder
	DefaultResponseMsgf(format string, args ...interface{}) HttpStatusErrorBuilder
	WithInternalErr(err error) HttpStatusErrorBuilder
	WithWrappedInternalErr(err error, msg string) HttpStatusErrorBuilder
	WithWrappedInternalErrf(err error, msg string, args ...interface{}) HttpStatusErrorBuilder
	BuildStatusError() *HttpStatusError
	Build() error
}

type httpStatusErrorBuilder struct {
	err *HttpStatusError
}

func HttpStatusErrorBuilderFromError(err error) HttpStatusErrorBuilder {
	var existing *HttpStatusError
	if errors.As(err, &existing) {
		return &httpStatusErrorBuilder{
			err: existing,
		}
	}

	return &httpStatusErrorBuilder{
		err: &HttpStatusError{
			InternalErr: err,
		},
	}
}
func NewHttpStatusErrorBuilder() HttpStatusErrorBuilder {
	return &httpStatusErrorBuilder{
		err: &HttpStatusError{
			Status: http.StatusInternalServerError,
		},
	}
}

func (b *httpStatusErrorBuilder) WithStatus(status int) HttpStatusErrorBuilder {
	b.err.Status = status
	return b
}

func (b *httpStatusErrorBuilder) WithStatusNotFound() HttpStatusErrorBuilder {
	return b.DefaultStatus(http.StatusNotFound)
}

func (b *httpStatusErrorBuilder) WithStatusBadRequest() HttpStatusErrorBuilder {
	return b.DefaultStatus(http.StatusBadRequest)
}

func (b *httpStatusErrorBuilder) WithStatusUnauthorized() HttpStatusErrorBuilder {
	return b.DefaultStatus(http.StatusUnauthorized)
}

func (b *httpStatusErrorBuilder) WithStatusForbidden() HttpStatusErrorBuilder {
	return b.DefaultStatus(http.StatusForbidden)
}

func (b *httpStatusErrorBuilder) WithStatusInternalServerError() HttpStatusErrorBuilder {
	return b.DefaultStatus(http.StatusInternalServerError)
}

func (b *httpStatusErrorBuilder) DefaultStatus(status int) HttpStatusErrorBuilder {
	if b.err.Status == 0 || b.err.Status == http.StatusInternalServerError {
		b.err.Status = status
	}

	return b
}

func (b *httpStatusErrorBuilder) DefaultStatusNotFound() HttpStatusErrorBuilder {
	return b.DefaultStatus(http.StatusNotFound)
}

func (b *httpStatusErrorBuilder) DefaultStatusBadRequest() HttpStatusErrorBuilder {
	return b.DefaultStatus(http.StatusBadRequest)
}

func (b *httpStatusErrorBuilder) DefaultStatusUnauthorized() HttpStatusErrorBuilder {
	return b.DefaultStatus(http.StatusUnauthorized)
}

func (b *httpStatusErrorBuilder) DefaultStatusForbidden() HttpStatusErrorBuilder {
	return b.DefaultStatus(http.StatusForbidden)
}

func (b *httpStatusErrorBuilder) WithResponseMsg(msg string) HttpStatusErrorBuilder {
	b.err.ResponseMsg = msg
	return b
}

func (b *httpStatusErrorBuilder) WithResponseMsgf(format string, args ...interface{}) HttpStatusErrorBuilder {
	b.err.ResponseMsg = fmt.Sprintf(format, args...)
	return b
}

func (b *httpStatusErrorBuilder) DefaultResponseMsg(msg string) HttpStatusErrorBuilder {
	if b.err.ResponseMsg == "" {
		b.err.ResponseMsg = msg
	}
	return b
}

func (b *httpStatusErrorBuilder) DefaultResponseMsgf(format string, args ...interface{}) HttpStatusErrorBuilder {
	return b.DefaultResponseMsg(fmt.Sprintf(format, args...))
}

func (b *httpStatusErrorBuilder) WithInternalErr(err error) HttpStatusErrorBuilder {
	var errStatusError *HttpStatusError
	if errors.As(err, &errStatusError) {
		if err == errStatusError {
			b.err = errStatusError
		} else {
			b.err.ResponseMsg = errStatusError.ResponseMsg
			b.err.Status = errStatusError.Status
			b.err.InternalErr = err
		}
	} else {
		b.err.InternalErr = err
	}

	return b
}

func (b *httpStatusErrorBuilder) WithWrappedInternalErr(err error, msg string) HttpStatusErrorBuilder {
	var errStatusError *HttpStatusError
	if errors.As(err, &errStatusError) {
		if err == errStatusError {
			b.err = errStatusError
		} else {
			b.err.ResponseMsg = errStatusError.ResponseMsg
			b.err.Status = errStatusError.Status
			b.err.InternalErr = err
		}
		b.err.InternalErr = errors.Wrap(b.err.InternalErr, msg)
	} else {
		b.err.InternalErr = errors.Wrap(err, msg)
	}

	return b
}

func (b *httpStatusErrorBuilder) WithWrappedInternalErrf(err error, msg string, args ...interface{}) HttpStatusErrorBuilder {
	return b.WithWrappedInternalErr(err, fmt.Sprintf(msg, args...))
}

func (b *httpStatusErrorBuilder) BuildStatusError() *HttpStatusError {
	return b.err
}

func (b *httpStatusErrorBuilder) Build() error {
	return b.BuildStatusError()
}

// HttpStatusErrorContains checks if the error contains the passed string. This is useful for checking if an error
// contains a specific string in the response message or internal error. This method will return false if if the passed
// error is not an HttpStatusError or it does not contain the specified string in either its internal error or response
// message. This is intended to be used in unit tests.
func HttpStatusErrorContains(err error, s string) bool {
	var he *HttpStatusError
	if errors.As(err, &he) {
		if strings.Contains(he.ResponseMsg, s) {
			return true
		}

		if he.InternalErr != nil && strings.Contains(he.InternalErr.Error(), s) {
			return true
		}
	}

	return false
}

// HttpStatusErrorIsStatusCode checks if the error is an HttpStatusError with the passed status code. This is intended
// to be used in unit tests.
func HttpStatusErrorIsStatusCode(err error, statusCode int) bool {
	var he *HttpStatusError
	return errors.As(err, &he) && he.Status == statusCode
}
