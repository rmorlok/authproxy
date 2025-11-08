package iface

import (
	"fmt"
	"net/http"
	"testing"

	mock "gopkg.in/h2non/gentleman-mock.v2"
	"gopkg.in/h2non/gentleman.v2"
	"gopkg.in/h2non/gock.v1"

	"github.com/stretchr/testify/assert"
)

func TestProxyRequest_Validate(t *testing.T) {
	tests := []struct {
		name          string
		proxyRequest  ProxyRequest
		expectedError string
	}{
		{
			name: "valid request",
			proxyRequest: ProxyRequest{
				URL:    "http://example.com",
				Method: http.MethodGet,
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
			},
			expectedError: "",
		},
		{
			name: "missing URL",
			proxyRequest: ProxyRequest{
				Method: http.MethodGet,
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
			},
			expectedError: "url is required",
		},
		{
			name: "missing Method",
			proxyRequest: ProxyRequest{
				URL: "http://example.com",
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
			},
			expectedError: "method is required",
		},
		{
			name: "invalid Method",
			proxyRequest: ProxyRequest{
				URL:    "http://example.com",
				Method: "INVALID",
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
			},
			expectedError: "invalid HTTP method",
		},
		{
			name: "missing Body for POST",
			proxyRequest: ProxyRequest{
				URL:    "http://example.com",
				Method: http.MethodPost,
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
			},
			expectedError: "either body_raw or body_json is must be specified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.proxyRequest.Validate()
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestProxyResponseFromGentlemen(t *testing.T) {
	tests := []struct {
		name          string
		setupMock     func() *gentleman.Response
		expected      *ProxyResponse
		expectedError error
	}{
		{
			name: "valid JSON response",
			setupMock: func() *gentleman.Response {
				defer gock.Off()
				mock.New("http://example.com").
					Get("/*").
					Reply(200).
					AddHeader("Content-Type", "application/json").
					BodyString(`{"key":"value"}`)

				client := gentleman.New()
				client.Use(mock.Plugin)

				req := client.Request()
				req.URL("http://example.com/some-path").Method("GET")
				resp, _ := req.Send()
				return resp
			},
			expected: &ProxyResponse{
				StatusCode: http.StatusOK,
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
				BodyJson: map[string]interface{}{"key": "value"},
			},
			expectedError: nil,
		},

		{
			name: "non-JSON response",
			setupMock: func() *gentleman.Response {
				defer gock.Off()
				mock.New("http://example.com").
					Get("/*").
					Reply(200).
					AddHeader("Content-Type", "text/plain").
					BodyString("this is not JSON")

				client := gentleman.New()
				client.Use(mock.Plugin)

				req := client.Request()
				req.URL("http://example.com/some-path").Method("GET")
				resp, _ := req.Send()
				return resp
			},
			expected: &ProxyResponse{
				StatusCode: http.StatusOK,
				Headers: map[string]string{
					"Content-Type": "text/plain",
				},
				BodyRaw: []byte("this is not JSON"),
			},
			expectedError: nil,
		},
		{
			name: "error response from server",
			setupMock: func() *gentleman.Response {
				defer gock.Off()
				mock.New("http://example.com").
					Get("/*").
					Reply(500).
					AddHeader("Content-Type", "application/json").
					BodyString(`{"error": "internal server error"}`)

				client := gentleman.New()
				client.Use(mock.Plugin)

				req := client.Request()
				req.URL("http://example.com/some-path").Method("GET")
				resp, _ := req.Send()
				return resp
			},
			expected: &ProxyResponse{
				StatusCode: http.StatusInternalServerError,
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
				BodyJson: map[string]interface{}{"error": "internal server error"},
			},
			expectedError: nil,
		},
		{
			name: "no response body",
			setupMock: func() *gentleman.Response {
				defer gock.Off()
				mock.New("http://example.com").
					Get("/*").
					Reply(204).
					AddHeader("Content-Type", "application/json")

				client := gentleman.New()
				client.Use(mock.Plugin)

				req := client.Request()
				req.URL("http://example.com/some-path").Method("GET")
				resp, _ := req.Send()
				return resp
			},
			expected: &ProxyResponse{
				StatusCode: http.StatusNoContent,
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
				BodyJson: nil,
			},
			expectedError: nil,
		},
		{
			name: "malformed JSON response",
			setupMock: func() *gentleman.Response {
				defer gock.Off()
				mock.New("http://example.com").
					Get("/*").
					Reply(200).
					AddHeader("Content-Type", "application/json").
					BodyString(`{"key": "value"`)

				client := gentleman.New()
				client.Use(mock.Plugin)

				req := client.Request()
				req.URL("http://example.com/some-path").Method("GET")
				resp, _ := req.Send()
				return resp
			},
			expected:      nil,
			expectedError: fmt.Errorf("unexpected EOF"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := tt.setupMock()
			actual, err := ProxyResponseFromGentlemen(resp)
			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, actual)
			}
		})
	}
}
