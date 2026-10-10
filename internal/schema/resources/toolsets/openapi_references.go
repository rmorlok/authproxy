package toolsets

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"unicode"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util"
	"gopkg.in/yaml.v3"
)

// OpenAPIReferenceConfig supplies document identities and in-memory reference
// content for a generation snapshot. URI identifiers never authorize fetching.
// Resolving references and checking their targets belong to the importer.
type OpenAPIReferenceConfig struct {
	// BaseURI supplies the entry document's retrieval context. When omitted,
	// the importer uses its retrieval URI. It does not override OpenAPI $self or
	// JSON Schema $id semantics. Inline documents may omit it when their refs
	// do not require an external base or the content supplies its own base.
	BaseURI *string `json:"baseUri,omitempty" yaml:"baseUri,omitempty"`

	// Bundle maps absolute, fragment-free document URIs to supplied content.
	// Each key supplies that document's retrieval context. Values remain opaque
	// JSON, including boolean schemas, until the importer checks target types.
	// An empty bundle and an omitted bundle both supply no additional documents.
	Bundle map[string]common.RawJSON `json:"bundle,omitempty" yaml:"bundle,omitempty"`
}

// Validate checks literal URI identities and JSON syntax without resolving
// references, interpreting document identifiers, or performing network/file I/O.
// A nil configuration preserves the document's existing reference context.
func (r *OpenAPIReferenceConfig) Validate(vc *common.ValidationContext) error {
	if r == nil {
		return nil
	}
	vc = validationContext(vc)
	var result *multierror.Error
	if r.BaseURI != nil {
		if err := validateOpenAPIReferenceURI(*r.BaseURI); err != nil {
			result = multierror.Append(result, vc.NewErrorForField("baseUri", err.Error()))
		}
	}
	// Stable ordering makes diagnostics reproducible for authored bundles.
	for _, uri := range slices.Sorted(maps.Keys(r.Bundle)) {
		if err := validateOpenAPIReferenceURI(uri); err != nil {
			result = multierror.Append(result, vc.NewErrorfForField("bundle", "document URI %q: %s", uri, err))
		}
		if !json.Valid(r.Bundle[uri]) {
			result = multierror.Append(result, vc.NewErrorfForField("bundle", "document %q must contain one JSON value", uri))
		}
	}
	return result.ErrorOrNil()
}

// validateOpenAPIReferenceURI checks a document identity, not a fetch URL.
// Non-HTTP schemes are allowed for supplied documents; fragments belong on
// references to locations inside a document, not on its retrieval identity.
func validateOpenAPIReferenceURI(value string) error {
	if strings.Contains(value, "{{") || strings.Contains(value, "}}") ||
		strings.ContainsFunc(value, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return fmt.Errorf("must be a literal URI without whitespace, control characters, or templates")
	}
	parsed, err := url.Parse(value)
	if err != nil || !parsed.IsAbs() {
		return fmt.Errorf("must be an absolute URI")
	}
	// Presence matters even for an empty fragment: URL.Parse discards a bare #.
	if parsed.User != nil || strings.Contains(value, "#") {
		return fmt.Errorf("must not contain user information or a fragment")
	}
	return nil
}

// Clone detaches the base pointer, bundle map, and each document's raw bytes.
// It retains invalid content and nil/empty states for subsequent diagnostics.
func (r *OpenAPIReferenceConfig) Clone() *OpenAPIReferenceConfig {
	if r == nil {
		return nil
	}
	clone := *r
	clone.BaseURI = util.CloneValue(r.BaseURI)
	clone.Bundle = maps.Clone(r.Bundle)
	for uri, raw := range clone.Bundle {
		clone.Bundle[uri] = slices.Clone(raw)
	}
	return &clone
}

// UnmarshalJSON rejects unknown, noncanonical, and null configuration fields
// and duplicate bundle identities. Bundle payloads remain arbitrary JSON.
// Failed decoding preserves the receiver rather than applying a partial update.
func (r *OpenAPIReferenceConfig) UnmarshalJSON(data []byte) error {
	fields, err := decodeStrictObject(data, "OpenAPI reference configuration", jsonFieldNames(reflect.TypeOf(OpenAPIReferenceConfig{}))...)
	if err != nil {
		return err
	}
	if raw, ok := fields["bundle"]; ok {
		if err := validateOpenAPIReferenceBundleJSON(raw); err != nil {
			return err
		}
	}
	// Decode exactly the checked fields so repeated top-level map fields cannot
	// merge earlier, unchecked documents into the resulting bundle.
	checked, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	type plain OpenAPIReferenceConfig
	var decoded plain
	if err := util.DecodeJSONStrict(checked, &decoded); err != nil {
		return err
	}
	*r = OpenAPIReferenceConfig(decoded)
	return nil
}

// validateOpenAPIReferenceBundleJSON checks a syntactically valid value from
// decodeStrictObject before ordinary map decoding can hide duplicate identities.
// URI escape spellings are compared as decoded JSON strings; payloads are opaque.
func validateOpenAPIReferenceBundleJSON(raw json.RawMessage) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token != json.Delim('{') {
		return fmt.Errorf("bundle must be an object")
	}
	seen := make(map[string]bool)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		uri := token.(string) // Object keys in valid JSON are always strings.
		if seen[uri] {
			return fmt.Errorf("bundle document URI %q must not be repeated", uri)
		}
		seen[uri] = true
		var document json.RawMessage
		if err := decoder.Decode(&document); err != nil {
			return err
		}
	}
	return nil
}

// UnmarshalYAML resolves aliases and merges before applying the JSON boundary,
// preserving opaque document values through the shared lossless conversion.
func (r *OpenAPIReferenceConfig) UnmarshalYAML(node *yaml.Node) error {
	var raw common.RawJSON
	if err := raw.UnmarshalYAML(node); err != nil {
		return err
	}
	return r.UnmarshalJSON(raw)
}
