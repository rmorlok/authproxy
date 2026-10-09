package toolsets

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/util"
	"gopkg.in/yaml.v3"
)

// OpenAPIDocument identifies one document to import into a generation snapshot.
// Exactly one of Inline and URL is required. Inline content remains opaque JSON;
// parsing OpenAPI versions, resolving references, and fetching are importer work.
type OpenAPIDocument struct {
	Inline common.RawJSON `json:"inline,omitempty" yaml:"inline,omitempty"`
	// URL is a literal acquisition address, independent of per-connection cfg.
	URL *string `json:"url,omitempty" yaml:"url,omitempty"`
	// FetchConnectionRef optionally authenticates the URL fetch. It does not
	// bind generated tools or authorize acquisition; the service resolves the
	// reference and checks administrative authority before using credentials.
	FetchConnectionRef *meta.ObjectReference `json:"fetchConnectionRef,omitempty" yaml:"fetchConnectionRef,omitempty"`
}

// Validate checks source exclusivity, literal acquisition URL syntax, and the
// optional connection reference shape. It performs no I/O and never interprets
// inline OpenAPI fields, JSON Schema dialects, or remote references.
func (d *OpenAPIDocument) Validate(vc *common.ValidationContext) error {
	vc = validationContext(vc)
	if d == nil {
		return vc.NewError("OpenAPI document is required")
	}
	var result *multierror.Error
	if (d.Inline == nil) == (d.URL == nil) {
		result = multierror.Append(result, vc.NewError("must contain exactly one of inline or url"))
	}
	if d.Inline != nil {
		raw := bytes.TrimSpace(d.Inline)
		// Checking syntax without decoding numbers keeps large integers and
		// arbitrary provider extensions intact for the later document parser.
		if !json.Valid(raw) || len(raw) == 0 || raw[0] != '{' {
			result = multierror.Append(result, vc.NewErrorForField("inline", "must contain one JSON object"))
		}
	}
	if d.URL != nil {
		if err := validateOpenAPIDocumentURL(*d.URL); err != nil {
			result = multierror.Append(result, vc.NewErrorForField("url", err.Error()))
		}
	}
	if d.FetchConnectionRef != nil {
		if d.URL == nil {
			result = multierror.Append(result, vc.NewErrorForField("fetchConnectionRef", "is only supported with url"))
		}
		// Acquisition can use a separately authorized connection in another
		// namespace; a Tool's same-namespace execution binding rule does not apply.
		result = multierror.Append(result, tools.ValidateConnectionReference(
			*d.FetchConnectionRef, "", vc.PushField("fetchConnectionRef"),
		))
	}
	return result.ErrorOrNil()
}

// validateOpenAPIDocumentURL rejects unresolved templates and credential-bearing
// or fragmented addresses. Errors omit the URL because its query may be private.
func validateOpenAPIDocumentURL(value string) error {
	if value == "" || strings.TrimSpace(value) != value ||
		strings.Contains(value, "{{") || strings.Contains(value, "}}") {
		return fmt.Errorf("must be a literal URL without surrounding whitespace or templates")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Opaque != "" || parsed.Hostname() == "" ||
		(!strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https")) {
		return fmt.Errorf("must be an absolute HTTP(S) URL with a host")
	}
	// Presence matters even for an empty fragment: URL.Parse discards a bare #.
	if parsed.User != nil || strings.Contains(value, "#") {
		return fmt.Errorf("must not contain user information or a fragment")
	}
	return nil
}

// Clone copies raw document bytes and reference pointers without serialization
// or normalization, retaining nil versus invalid empty content for validation.
func (d *OpenAPIDocument) Clone() *OpenAPIDocument {
	if d == nil {
		return nil
	}
	clone := *d
	clone.Inline = slices.Clone(d.Inline)
	clone.URL = util.CloneValue(d.URL)
	clone.FetchConnectionRef = util.CloneValue(d.FetchConnectionRef)
	return &clone
}

// MarshalJSON preserves an explicitly supplied inline source, even when its raw
// bytes are invalid or empty. Dropping it via omitempty could turn an invalid
// two-source document into a valid URL-only acquisition request.
func (d OpenAPIDocument) MarshalJSON() ([]byte, error) {
	var inline *common.RawJSON
	if d.Inline != nil {
		inline = &d.Inline
	}
	return json.Marshal(struct {
		Inline             *common.RawJSON       `json:"inline,omitempty"`
		URL                *string               `json:"url,omitempty"`
		FetchConnectionRef *meta.ObjectReference `json:"fetchConnectionRef,omitempty"`
	}{inline, d.URL, d.FetchConnectionRef})
}

// MarshalYAML applies the same source-presence rules as JSON while retaining
// native numeric nodes in opaque inline document content.
func (d OpenAPIDocument) MarshalYAML() (any, error) {
	raw, err := d.MarshalJSON()
	if err != nil {
		return nil, err
	}
	return common.RawJSON(raw).MarshalYAML()
}

// UnmarshalJSON accepts only canonical owned fields and rejects explicit nulls.
// Fields inside inline remain arbitrary document data. Assignment is atomic so
// an invalid replacement cannot partially change an existing acquisition source.
func (d *OpenAPIDocument) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := util.DecodeJSONStrict(data, &fields); err != nil {
		return err
	}
	if fields == nil {
		return fmt.Errorf("OpenAPI document must be an object")
	}
	var decoded OpenAPIDocument
	for field, raw := range fields {
		var destination any
		switch field {
		case "inline":
			destination = &decoded.Inline
		case "url":
			destination = &decoded.URL
		case "fetchConnectionRef":
			destination = &decoded.FetchConnectionRef
		default:
			return fmt.Errorf("unknown OpenAPI document field %q", field)
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("%s must not be null", field)
		}
		if field == "fetchConnectionRef" {
			if err := validateOpenAPIFetchReferenceFields(raw); err != nil {
				return err
			}
		}
		if err := util.DecodeJSONStrict(raw, destination); err != nil {
			return fmt.Errorf("decode %s: %w", field, err)
		}
	}
	*d = decoded
	return nil
}

// UnmarshalYAML resolves aliases and merges before the JSON boundary so null
// source fields and reference keys cannot disappear through pointer decoding.
func (d *OpenAPIDocument) UnmarshalYAML(node *yaml.Node) error {
	var raw common.RawJSON
	if err := raw.UnmarshalYAML(node); err != nil {
		return err
	}
	return d.UnmarshalJSON(raw)
}

// validateOpenAPIFetchReferenceFields checks raw reference field presence before
// typed decoding. Connections have no generation, including explicit zero/null;
// other fields must use their canonical spelling and cannot be explicitly null.
func validateOpenAPIFetchReferenceFields(data []byte) error {
	var fields map[string]json.RawMessage
	if err := util.DecodeJSONStrict(data, &fields); err != nil {
		return err
	}
	for field, raw := range fields {
		switch field {
		case "apiVersion", "kind", "id", "name", "namespace":
		case "generation":
			return fmt.Errorf("fetchConnectionRef.generation does not apply to connections")
		default:
			return fmt.Errorf("unknown fetchConnectionRef field %q", field)
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("fetchConnectionRef.%s must not be null", field)
		}
	}
	return nil
}
