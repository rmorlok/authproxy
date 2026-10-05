package iface

import (
	"bytes"
	"context"
	"net/http"
	"strings"

	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/schema/common"
	smeta "github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"gopkg.in/h2non/gentleman.v2"
)

type HeadersVal = common.HeadersVal

type ProxyRequest struct {
	URL      string                `json:"url"`
	Method   string                `json:"method"`
	Headers  map[string]HeadersVal `json:"headers"`
	Labels   map[string]string     `json:"labels,omitempty"`
	BodyRaw  []byte                `json:"bodyRaw,omitempty"`
	BodyJson interface{}           `json:"bodyJson,omitempty"`
}

func (r *ProxyRequest) Apply(req *gentleman.Request) {
	req.URL(r.URL)
	req.Method(r.Method)

	for h, v := range r.Headers {
		for _, hv := range v.Values() {
			req.AddHeader(h, hv)
		}
	}

	if r.BodyJson != nil {
		req.JSON(r.BodyJson)
	} else {
		req.Body(bytes.NewReader(r.BodyRaw))
	}
}

func (r *ProxyRequest) Validate() error {
	errors := make([]string, 0)

	if r.URL == "" {
		errors = append(errors, "url is required")
	}

	if r.Method == "" {
		errors = append(errors, "method is required")
	}

	validMethods := map[string]bool{
		http.MethodGet:     true,
		http.MethodHead:    true,
		http.MethodPost:    true,
		http.MethodPut:     true,
		http.MethodPatch:   true,
		http.MethodDelete:  true,
		http.MethodConnect: true,
		http.MethodOptions: true,
		http.MethodTrace:   true,
	}

	if !validMethods[r.Method] {
		errors = append(errors, "invalid HTTP method")
	}

	if (r.Method == http.MethodPut || r.Method == http.MethodPost || r.Method == http.MethodPatch) &&
		r.BodyJson == nil && r.BodyRaw == nil {
		errors = append(errors, "either body_raw or body_json is must be specified")
	}

	if r.Labels != nil {
		if err := smeta.ValidateUserLabels(r.Labels); err != nil {
			errors = append(errors, "invalid labels: "+err.Error())
		}
	}

	if len(errors) > 0 {
		return httperr.BadRequest(strings.Join(errors, ", "))
	}

	return nil
}

type ProxyResponse struct {
	StatusCode int               `json:"statusCode"`
	Headers    map[string]string `json:"headers"`
	BodyRaw    []byte            `json:"bodyRaw"`
	BodyJson   interface{}       `json:"bodyJson"`
}

// ProxyResponseFromGentlemen creates a ProxyResponse from a gentleman.Response
func ProxyResponseFromGentlemen(resp *gentleman.Response) (*ProxyResponse, error) {
	proxyResp := &ProxyResponse{
		StatusCode: resp.StatusCode,
		Headers:    make(map[string]string),
	}

	for name, values := range resp.Header {
		if len(values) > 0 {
			if len(values) > 1 {
				proxyResp.Headers[name] = strings.Join(values, ", ")
			} else {
				proxyResp.Headers[name] = values[0]
			}
		}
	}

	// Optionally parse BodyJson if content-type is JSON
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		var bodyJson interface{}
		err := resp.JSON(&bodyJson)
		if err != nil {
			return nil, err
		}
		proxyResp.BodyJson = bodyJson
	} else {
		proxyResp.BodyRaw = resp.Bytes()
	}

	return proxyResp, nil
}

// RawProxyRequest carries the inputs to the streaming proxy paths.
// Separate from ProxyRequest because raw bodies are streamed (io.Reader on
// the outbound request) rather than buffered into BodyRaw/BodyJson, and
// the URL and headers are prepared by the caller.
type RawProxyRequest struct {
	// Outbound is the request the orchestrator will send to the upstream.
	// Its URL and headers are cloned before credentials are applied. Calling
	// ProxyRequestStream or ProxyRequestRaw transfers ownership of Body: it
	// is closed even if credential resolution fails before an HTTP request
	// can be sent. The caller must not read or reuse that body concurrently.
	// Callers forwarding an inbound request also resolve the upstream URL
	// and filter hop-by-hop headers before constructing this request.
	// Requests use httpf's instrumented *http.Client without body buffering.
	Outbound *http.Request
	// Labels are forwarded to httpf's ForLabels chain (rate-limit,
	// request events, OTel) — same shape as ProxyRequest.Labels for the
	// wrapped path.
	Labels map[string]string
}

type Proxy interface {
	ProxyRequest(ctx context.Context, reqType httpf.RequestType, req *ProxyRequest) (*ProxyResponse, error)
	// ProxyRequestStream returns response headers and a streaming body without
	// consuming it. On success the caller must close the response body; early
	// Close does not drain the stream. Only bodyless requests may be retried
	// once after credential recovery from an upstream 401. See RawProxyRequest
	// for ownership of the outgoing request body, including failure paths.
	ProxyRequestStream(ctx context.Context, reqType httpf.RequestType, req *RawProxyRequest) (*http.Response, error)
	ProxyRequestRaw(ctx context.Context, reqType httpf.RequestType, req *RawProxyRequest, w http.ResponseWriter) error
}
