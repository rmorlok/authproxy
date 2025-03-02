package api_common

import (
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHttpStatusError_Error(t *testing.T) {
	tests := []struct {
		name          string
		err           *HttpStatusError
		expectedError string
	}{
		{"onlyInternalErr", &HttpStatusError{InternalErr: errors.New("internal error")}, "internal error"},
		{"onlyResponseMsg", &HttpStatusError{ResponseMsg: "response message"}, "response message"},
		{"onlyStatus", &HttpStatusError{Status: http.StatusNotFound}, "HTTP 404: Not Found"},
		{"noDetails", &HttpStatusError{}, "Unknown error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.expectedError {
				t.Errorf("Error() = %v, want %v", got, tt.expectedError)
			}
		})
	}
}

func TestHttpStatusError_ResponseMsgOrDefault(t *testing.T) {
	tests := []struct {
		name             string
		err              *HttpStatusError
		expectedResponse string
	}{
		{"withResponseMsg", &HttpStatusError{ResponseMsg: "response message"}, "response message"},
		{"withStatus", &HttpStatusError{Status: http.StatusUnauthorized}, "Unauthorized"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.ResponseMsgOrDefault(); got != tt.expectedResponse {
				t.Errorf("ResponseMsgOrDefault() = %v, want %v", got, tt.expectedResponse)
			}
		})
	}
}

func TestHttpStatusError_WriteGinResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name           string
		err            *HttpStatusError
		debug          bool
		expectedStatus int
		expectedBody   string
	}{
		{
			"normalMode",
			&HttpStatusError{Status: http.StatusForbidden, ResponseMsg: "Forbidden"},
			false,
			http.StatusForbidden,
			`{"error":"Forbidden"}`,
		},
		{
			"debug",
			&HttpStatusError{Status: http.StatusForbidden, ResponseMsg: "Forbidden", InternalErr: errors.New("internal error text")},
			true,
			http.StatusForbidden,
			`{"error":"internal error text"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &mockDebuggable{debug: tt.debug}
			rec := httptest.NewRecorder()
			gctx, _ := gin.CreateTestContext(rec)

			tt.err.WriteGinResponse(cfg, gctx)

			if rec.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rec.Code)
			}

			trimmedBody := strings.TrimSpace(rec.Body.String())
			if trimmedBody != tt.expectedBody {
				t.Errorf("expected body %q, got %q", tt.expectedBody, trimmedBody)
			}
		})
	}
}

func TestHttpStatusError_WriteResponse(t *testing.T) {
	tests := []struct {
		name           string
		err            *HttpStatusError
		debug          bool
		expectedStatus int
		expectedBody   string
	}{
		{
			"normalMode",
			&HttpStatusError{Status: http.StatusNotFound, ResponseMsg: "Not Found"},
			false,
			http.StatusNotFound,
			`{"error":"Not Found"}`,
		},
		{
			"debug",
			&HttpStatusError{Status: http.StatusNotFound, ResponseMsg: "Not Found", InternalErr: errors.New("internal error text")},
			true,
			http.StatusNotFound,
			`{"error":"internal error text"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &mockDebuggable{debug: tt.debug}
			rec := httptest.NewRecorder()

			tt.err.WriteResponse(cfg, rec)

			if rec.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rec.Code)
			}

			if rec.Body.String() != tt.expectedBody {
				t.Errorf("expected body %q, got %q", tt.expectedBody, rec.Body.String())
			}
		})
	}
}

func TestHttpStatusErrorBuilder(t *testing.T) {
	tests := []struct {
		name      string
		builderFn func(HttpStatusErrorBuilder) *HttpStatusError
		expected  *HttpStatusError
	}{
		{
			"with status",
			func(b HttpStatusErrorBuilder) *HttpStatusError {
				return b.WithStatus(http.StatusBadRequest).BuildStatusError()
			},
			&HttpStatusError{Status: http.StatusBadRequest},
		},
		{
			"with response message",
			func(b HttpStatusErrorBuilder) *HttpStatusError {
				return b.WithResponseMsg("Some error").BuildStatusError()
			},
			&HttpStatusError{ResponseMsg: "Some error", Status: http.StatusInternalServerError},
		},
		{
			"with internal error",
			func(b HttpStatusErrorBuilder) *HttpStatusError {
				return b.WithInternalErr(errors.New("internal error")).BuildStatusError()
			},
			&HttpStatusError{InternalErr: errors.New("internal error"), Status: http.StatusInternalServerError},
		},
		{
			"with wrapped internal error",
			func(b HttpStatusErrorBuilder) *HttpStatusError {
				return b.WithWrappedInternalErr(errors.New("base error"), "wrapped error").BuildStatusError()
			},
			&HttpStatusError{InternalErr: errors.New("wrapped error: base error"), Status: http.StatusInternalServerError},
		},
		{
			"with internal http status error",
			func(b HttpStatusErrorBuilder) *HttpStatusError {
				return b.WithInternalErr(&HttpStatusError{
					ResponseMsg: "ResponseMsg",
					InternalErr: errors.New("InternalErr"),
					Status:      http.StatusTeapot,
				}).BuildStatusError()
			},
			&HttpStatusError{
				ResponseMsg: "ResponseMsg",
				InternalErr: errors.New("InternalErr"),
				Status:      http.StatusTeapot,
			},
		},
		{
			"with wrapped http status internal error",
			func(b HttpStatusErrorBuilder) *HttpStatusError {
				return b.WithWrappedInternalErr(&HttpStatusError{
					ResponseMsg: "ResponseMsg",
					InternalErr: errors.New("InternalErr"),
					Status:      http.StatusTeapot,
				}, "foo - bar").BuildStatusError()
			},
			&HttpStatusError{
				ResponseMsg: "ResponseMsg",
				InternalErr: errors.New("foo - bar: InternalErr"),
				Status:      http.StatusTeapot,
			},
		},
		{
			"with wrapped http status internal error format",
			func(b HttpStatusErrorBuilder) *HttpStatusError {
				return b.WithWrappedInternalErrf(&HttpStatusError{
					ResponseMsg: "ResponseMsg",
					InternalErr: errors.New("InternalErr"),
					Status:      http.StatusTeapot,
				}, "foo - %s", "bar").BuildStatusError()
			},
			&HttpStatusError{
				ResponseMsg: "ResponseMsg",
				InternalErr: errors.New("foo - bar: InternalErr"),
				Status:      http.StatusTeapot,
			},
		},
		{
			"with recursively nested internal http status error",
			func(b HttpStatusErrorBuilder) *HttpStatusError {
				return b.WithInternalErr(errors.Wrapf(&HttpStatusError{
					ResponseMsg: "ResponseMsg",
					InternalErr: errors.New("InternalErr"),
					Status:      http.StatusTeapot,
				}, "outer")).BuildStatusError()
			},
			&HttpStatusError{
				ResponseMsg: "ResponseMsg",
				InternalErr: errors.New("outer: InternalErr"),
				Status:      http.StatusTeapot,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewHttpStatusErrorBuilder()
			result := tt.builderFn(builder)

			if result.Status != tt.expected.Status {
				t.Errorf("expected status %d, got %d", tt.expected.Status, result.Status)
			}

			if result.ResponseMsg != tt.expected.ResponseMsg {
				t.Errorf("expected response message %q, got %q", tt.expected.ResponseMsg, result.ResponseMsg)
			}

			if result.InternalErr != nil && result.InternalErr.Error() != tt.expected.InternalErr.Error() {
				t.Errorf("expected internal error %q, got %q", tt.expected.InternalErr, result.InternalErr)
			}
		})
	}
}

func TestAsHttpStatusError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected *HttpStatusError
	}{
		{
			"nil error",
			nil,
			&HttpStatusError{Status: http.StatusInternalServerError},
		},
		{
			"plain error",
			errors.New("plain error"),
			&HttpStatusError{
				InternalErr: errors.New("plain error"),
				Status:      http.StatusInternalServerError,
			},
		},
		{
			"status error wrapped",
			errors.Wrap(&HttpStatusError{
				Status:      http.StatusBadRequest,
				ResponseMsg: "response message",
			}, "wrapped"),
			&HttpStatusError{
				Status:      http.StatusBadRequest,
				ResponseMsg: "response message",
				InternalErr: errors.New("wrapped: response message"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AsHttpStatusError(tt.err)

			if got.Status != tt.expected.Status {
				t.Errorf("expected status %d, got %d", tt.expected.Status, got.Status)
			}
			if got.ResponseMsg != tt.expected.ResponseMsg {
				t.Errorf("expected response message %q, got %q", tt.expected.ResponseMsg, got.ResponseMsg)
			}
			if got.InternalErr != nil && got.InternalErr.Error() != tt.expected.InternalErr.Error() {
				t.Errorf("expected internal error %q, got %q", tt.expected.InternalErr, got.InternalErr)
			}
		})
	}
}
