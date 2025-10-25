package connectors

import (
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/config/common"
)

// ProbeHttp is a definition for an HTTP probe. This can either be used as a auth proxy request or a raw HTTP
// request.
type ProbeHttp struct {
	// Method is the HTTP method to use.
	Method string `json:"method" yaml:"method"`

	// Url is the URL to make the request to.
	URL string `json:"url" yaml:"url"`

	// Headers are the headers to send with the request.
	Headers map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`

	// BodyRaw is the raw body to send with the request. This will expect a base64 encoded string.
	BodyRaw []byte `json:"body_raw,omitempty" yaml:"body_raw,omitempty"`

	// BodyJson is the JSON body to send with the request. If used, the config can specify an inlined object.
	BodyJson interface{} `json:"body_json,omitempty" yaml:"body_json,omitempty"`

	// Body is the body to send with the request. This will be a string value.
	Body string `json:"body,omitempty" yaml:"body,omitempty"`

	// Timeout is the timeout for the request. Defaults to 60 seconds.
	Timeout *common.HumanDuration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

func (p *ProbeHttp) GetTimeoutOrDefault() time.Duration {
	if p == nil || p.Timeout == nil {
		return 60 * time.Second
	}

	return p.Timeout.Duration
}

func (p *ProbeHttp) Validate(vc *common.ValidationContext) error {
	result := &multierror.Error{}

	if p.Method == "" {
		result = multierror.Append(result, vc.NewErrorfForField("method", "method is required"))
	}
	if p.URL == "" {
		result = multierror.Append(result, vc.NewErrorfForField("url", "url is required"))
	}

	bodyCount := 0
	if p.BodyRaw != nil {
		bodyCount++
	}
	if p.BodyJson != nil {
		bodyCount++
	}
	if p.Body != "" {
		bodyCount++
	}
	if bodyCount > 1 {
		result = multierror.Append(result, vc.NewError("only one of body_raw, body_json, or body can be specified"))
	}

	return result.ErrorOrNil()
}
