package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util"
	"gopkg.in/yaml.v3"
)

// ProxyHTTP describes one connection-authenticated HTTP request and its result
// mapping. Strings may contain tool templates; rendering and final request
// validation occur before execution. Credentials retain proxy precedence.
type ProxyHTTP struct {
	Method  string            `json:"method" yaml:"method"`
	URL     string            `json:"url" yaml:"url"`
	Headers map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`

	// Query contains unencoded scalar or repeated scalar values. A single-key
	// $value object inserts a typed value from invocation context at runtime.
	Query map[string]common.RawJSON `json:"query,omitempty" yaml:"query,omitempty"`

	//
	// At most one body mode may be present. RawJSON retains explicit JSON null
	// as a body, while nil means no bodyJson mode was supplied.
	//

	BodyJSON     common.RawJSON `json:"bodyJson,omitempty" yaml:"bodyJson,omitempty"`
	BodyTemplate *BodyTemplate  `json:"bodyTemplate,omitempty" yaml:"bodyTemplate,omitempty"`
	// BodyRaw is base64, optionally produced by a string template. It never
	// denotes a server filesystem path; headers supply its media type.
	BodyRaw   *string        `json:"bodyRaw,omitempty" yaml:"bodyRaw,omitempty"`
	Form      *FormBody      `json:"form,omitempty" yaml:"form,omitempty"`
	Multipart *MultipartBody `json:"multipart,omitempty" yaml:"multipart,omitempty"`

	Response *HTTPResponse `json:"response,omitempty" yaml:"response,omitempty"`

	// nullBodyFields preserves malformed explicit nulls that pointer decoding
	// would otherwise mistake for an omitted non-JSON body mode.
	nullBodyFields []string
}

// BodyTemplate supplies rendered text and its declared media type. A JSON
// media type requires parsing the rendered result as JSON before sending it.
type BodyTemplate struct {
	MediaType string `json:"mediaType" yaml:"mediaType"`
	Template  string `json:"template" yaml:"template"`
}

// FormBody supplies application/x-www-form-urlencoded fields. Values use the
// same scalar/repeated-value encoding as structured query parameters.
type FormBody struct {
	Fields map[string]common.RawJSON `json:"fields" yaml:"fields"`
}

// MultipartBody is an ordered set of ordinary multipart fields or files.
// Repeated names remain separate parts; streaming and nested multipart are
// outside this authored contract.
type MultipartBody struct {
	Parts []MultipartPart `json:"parts" yaml:"parts"`
}

// MultipartPart contains either text or base64 binary data. Filename and
// MediaType describe a part and never cause server filesystem access.
type MultipartPart struct {
	Name      string  `json:"name" yaml:"name"`
	Filename  string  `json:"filename,omitempty" yaml:"filename,omitempty"`
	MediaType string  `json:"mediaType,omitempty" yaml:"mediaType,omitempty"`
	Text      *string `json:"text,omitempty" yaml:"text,omitempty"`
	BodyRaw   *string `json:"bodyRaw,omitempty" yaml:"bodyRaw,omitempty"`
}

// HTTPResponse optionally replaces passthrough output with a pure JavaScript
// expression. Error mappings run first; absent a matching rule, non-2xx
// responses are tool errors. The transform has no network access.
type HTTPResponse struct {
	TransformJavascript *string         `json:"transformJavascript,omitempty" yaml:"transformJavascript,omitempty"`
	Errors              []HTTPErrorRule `json:"errors,omitempty" yaml:"errors,omitempty"`
}

// HTTPErrorRule maps selected HTTP statuses to a safe administrator-authored
// error. Exactly one selector applies: Statuses, StatusClass, or Default.
// Exact statuses outrank classes, which outrank the default. Overlaps at one
// priority are invalid. Retryable is advice and never triggers an automatic
// invocation retry or proves that repeating an operation is safe.
type HTTPErrorRule struct {
	Statuses    []int  `json:"statuses,omitempty" yaml:"statuses,omitempty"`
	StatusClass *int   `json:"statusClass,omitempty" yaml:"statusClass,omitempty"`
	Default     bool   `json:"default,omitempty" yaml:"default,omitempty"`
	Code        string `json:"code" yaml:"code"`
	Message     string `json:"message" yaml:"message"`
	Retryable   *bool  `json:"retryable,omitempty" yaml:"retryable,omitempty"`
}

// UnmarshalJSON preserves explicit null body modes while retaining strict
// decoding for all fields owned by the HTTP contract.
func (p *ProxyHTTP) UnmarshalJSON(data []byte) error {
	type plain ProxyHTTP
	var decoded plain

	if err := util.DecodeJSONStrict(data, &decoded); err != nil {
		return err
	}

	var fields map[string]json.RawMessage

	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}

	*p = ProxyHTTP(decoded)
	for field, raw := range fields {
		if strings.TrimSpace(string(raw)) != "null" {
			continue
		}

		switch strings.ToLower(field) {
		case "bodytemplate", "bodyraw", "form", "multipart":
			p.nullBodyFields = append(p.nullBodyFields, field)
		}
	}

	return nil
}

// UnmarshalYAML translates the full YAML value before strict JSON decoding so
// explicit nulls survive, including values introduced by aliases or merges.
// YAML otherwise skips custom unmarshalling for null nodes and loses presence.
func (p *ProxyHTTP) UnmarshalYAML(node *yaml.Node) error {
	var raw common.RawJSON
	if err := raw.UnmarshalYAML(node); err != nil {
		return err
	}
	return p.UnmarshalJSON(raw)
}

// UnmarshalJSON distinguishes a required empty template from an omitted or
// null field. A native BodyTemplate value still serializes an empty template.
func (b *BodyTemplate) UnmarshalJSON(data []byte) error {
	var wire struct {
		MediaType string  `json:"mediaType"`
		Template  *string `json:"template"`
	}

	if err := util.DecodeJSONStrict(data, &wire); err != nil {
		return err
	}

	if wire.Template == nil {
		return fmt.Errorf("bodyTemplate.template is required and must not be null")
	}

	*b = BodyTemplate{MediaType: wire.MediaType, Template: *wire.Template}

	return nil
}

// UnmarshalYAML applies the same template-presence policy as the JSON contract.
func (b *BodyTemplate) UnmarshalYAML(node *yaml.Node) error {
	var raw common.RawJSON

	if err := raw.UnmarshalYAML(node); err != nil {
		return err
	}

	return b.UnmarshalJSON(raw)
}

// UnmarshalJSON rejects explicit null part modes instead of treating them as
// omitted and silently choosing the other mode.
func (p *MultipartPart) UnmarshalJSON(data []byte) error {
	type plain MultipartPart
	var decoded plain

	if err := util.DecodeJSONStrict(data, &decoded); err != nil {
		return err
	}

	var fields map[string]json.RawMessage

	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}

	for field, raw := range fields {
		if (strings.EqualFold(field, "text") || strings.EqualFold(field, "bodyRaw")) &&
			strings.TrimSpace(string(raw)) == "null" {
			return fmt.Errorf("multipart part %s must not be null", field)
		}
	}

	*p = MultipartPart(decoded)

	return nil
}

// UnmarshalYAML applies the same part-mode policy as the JSON contract.
func (p *MultipartPart) UnmarshalYAML(node *yaml.Node) error {
	var raw common.RawJSON

	if err := raw.UnmarshalYAML(node); err != nil {
		return err
	}

	return p.UnmarshalJSON(raw)
}

// UnmarshalJSON rejects explicit null selectors and default:false. Otherwise
// decoding would erase their presence and silently select a different rule.
func (r *HTTPErrorRule) UnmarshalJSON(data []byte) error {
	type plain HTTPErrorRule
	var decoded plain

	if err := util.DecodeJSONStrict(data, &decoded); err != nil {
		return err
	}

	var fields map[string]json.RawMessage

	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}

	for field, raw := range fields {
		switch strings.ToLower(field) {
		case "statuses", "statusclass":
			if strings.TrimSpace(string(raw)) == "null" {
				return fmt.Errorf("HTTP error rule %s must not be null", field)
			}
		case "default":
			if !decoded.Default {
				return fmt.Errorf("HTTP error rule default must be true when supplied")
			}
		}
	}

	*r = HTTPErrorRule(decoded)
	return nil
}

// UnmarshalYAML applies the same selector-presence policy as JSON decoding.
func (r *HTTPErrorRule) UnmarshalYAML(node *yaml.Node) error {
	var raw common.RawJSON
	if err := raw.UnmarshalYAML(node); err != nil {
		return err
	}
	return r.UnmarshalJSON(raw)
}
