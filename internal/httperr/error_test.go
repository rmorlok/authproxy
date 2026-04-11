package httperr

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/stretchr/testify/require"
)

func TestError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *Error
		expected string
	}{
		{"onlyInternalErr", &Error{InternalErr: errors.New("internal error")}, "internal error"},
		{"onlyResponseMsg", &Error{ResponseMsg: "response message"}, "response message"},
		{"onlyStatus", &Error{Status: http.StatusNotFound}, "HTTP 404: Not Found"},
		{"noDetails", &Error{}, "Unknown error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.err.Error())
		})
	}
}

func TestError_Unwrap(t *testing.T) {
	inner := errors.New("inner")
	err := &Error{Status: 500, InternalErr: inner}
	require.True(t, errors.Is(err, inner))
	require.Nil(t, (&Error{}).Unwrap())
}

func TestError_ResponseMsgOrDefault(t *testing.T) {
	tests := []struct {
		name     string
		err      *Error
		expected string
	}{
		{"withResponseMsg", &Error{ResponseMsg: "response message"}, "response message"},
		{"withStatus", &Error{Status: http.StatusUnauthorized}, "Unauthorized"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.err.ResponseMsgOrDefault())
		})
	}
}

func TestError_WriteResponse(t *testing.T) {
	tests := []struct {
		name              string
		err               *Error
		debug             bool
		expectedStatus    int
		expectedBodyRegex string
	}{
		{
			"normalMode",
			&Error{Status: http.StatusNotFound, ResponseMsg: "Not Found"},
			false,
			http.StatusNotFound,
			`{"error":"Not Found"}`,
		},
		{
			"debug",
			&Error{Status: http.StatusNotFound, ResponseMsg: "Not Found", InternalErr: errors.New("internal error text")},
			true,
			http.StatusNotFound,
			`{"error":"Not Found","stack_trace":"internal error text.*"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := apctx.WithDebugMode(context.Background(), tt.debug)
			rec := httptest.NewRecorder()

			tt.err.WriteResponse(ctx, nil, rec)

			require.Equal(t, tt.expectedStatus, rec.Code)

			trimmedBody := strings.TrimSpace(rec.Body.String())
			matched, err := regexp.MatchString(tt.expectedBodyRegex, trimmedBody)
			require.NoError(t, err)
			require.True(t, matched, "expected body to match regex %q, but got %q", tt.expectedBodyRegex, trimmedBody)
		})
	}
}
