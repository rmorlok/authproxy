package proxy

import (
	"bytes"
	"github.com/rmorlok/authproxy/api_common"
	"github.com/rmorlok/authproxy/context"
	"gopkg.in/h2non/gentleman.v2"
	"net/http"
	"strings"
)

type ProxyRequest struct {
	URL      string            `json:"url"`
	Method   string            `json:"method"`
	Headers  map[string]string `json:"headers"`
	BodyRaw  []byte            `json:"body_raw,omitempty"`
	BodyJson interface{}       `json:"body_json,omitempty"`
}

func (r *ProxyRequest) Apply(req *gentleman.Request) {
	req.URL(r.URL)
	req.Method(r.Method)

	for h, v := range r.Headers {
		req.AddHeader(h, v)
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

	if len(errors) > 0 {
		return api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg(strings.Join(errors, ", ")).
			Build()
	}

	return nil
}

type ProxyResponse struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	BodyRaw    []byte            `json:"body_raw"`
	BodyJson   interface{}       `json:"body_json"`
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

type Proxy interface {
	ProxyRequest(ctx context.Context, req *ProxyRequest) (*ProxyResponse, error)
	ProxyRequestRaw(ctx context.Context, req *ProxyRequest, w http.ResponseWriter) error
}
