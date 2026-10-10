package toolsets

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util"
	"gopkg.in/yaml.v3"
)

// OpenAPIServerConfig chooses either a document server or a deliberate URL
// override. Exactly one of Index and URL is required when this object is supplied.
// Omitting the object preserves the importer's default server selection.
type OpenAPIServerConfig struct {
	// URL replaces the document-selected server base and may use connection cfg
	// templates, including a whole-URL template. Compilation and rendered URL
	// validation happen later, before execution through the bound connection.
	URL *string `json:"url,omitempty" yaml:"url,omitempty"`

	// Index is a zero-based index into each operation's effective OAS 3 servers
	// list, after operation/path/document precedence. It is not an index into a
	// merged list and does not fall back to a parent list when out of range.
	// Explicit zero must survive serialization; document-dependent checks and
	// diagnostics for formats without servers lists belong to the importer.
	Index *int `json:"index,omitempty" yaml:"index,omitempty"`

	// Variables binds document server variables to literal import-time strings.
	// Unbound variables retain document defaults. The importer checks names,
	// enums, defaults, and relative URL resolution against each selected server.
	// Bindings require Index and are never evaluated as templates; use URL for
	// runtime cfg substitution. Empty strings and template-looking text remain
	// literal values for the importer to check against the document.
	// A pointer preserves supplied {}, including invalid URL-plus-variables
	// combinations, rather than dropping them during ordinary serialization.
	Variables *map[string]string `json:"variables,omitempty" yaml:"variables,omitempty"`
}

// Validate checks authored mode, index, URL, and literal variable shapes without
// parsing the document, choosing servers, rendering templates, or performing I/O.
// A nil configuration leaves server selection to the importer and is valid.
func (s *OpenAPIServerConfig) Validate(vc *common.ValidationContext) error {
	if s == nil {
		return nil
	}
	vc = validationContext(vc)
	var result *multierror.Error
	if (s.URL == nil) == (s.Index == nil) {
		result = multierror.Append(result, vc.NewError("must contain exactly one of url or index"))
	}
	if s.URL != nil {
		if strings.TrimSpace(*s.URL) == "" {
			result = multierror.Append(result, vc.NewErrorForField("url", "must not be blank"))
		} else if strings.TrimSpace(*s.URL) != *s.URL || strings.ContainsAny(*s.URL, "\r\n") {
			result = multierror.Append(result, vc.NewErrorForField("url", "must not contain surrounding whitespace or newlines"))
		}
	}
	if s.Index != nil && *s.Index < 0 {
		result = multierror.Append(result, vc.NewErrorForField("index", "must not be negative"))
	}
	if s.Variables != nil {
		if s.URL != nil {
			result = multierror.Append(result, vc.NewErrorForField("variables", "is only supported with index, not a URL override"))
		} else if s.Index == nil {
			result = multierror.Append(result, vc.NewErrorForField("variables", "requires index"))
		}
		if *s.Variables == nil {
			result = multierror.Append(result, vc.NewErrorForField("variables", "must be an object, not null"))
		}
		// Sort names so diagnostics remain stable despite Go map iteration order.
		for _, name := range slices.Sorted(maps.Keys(*s.Variables)) {
			if strings.TrimSpace(name) == "" {
				result = multierror.Append(result, vc.NewErrorfForField("variables", "name %q must not be blank", name))
			}
		}
	}
	return result.ErrorOrNil()
}

// Clone detaches every pointer and the bindings map without normalization,
// preserving omitted, empty, and invalid values for subsequent diagnostics.
func (s *OpenAPIServerConfig) Clone() *OpenAPIServerConfig {
	if s == nil {
		return nil
	}
	clone := *s
	clone.URL = util.CloneValue(s.URL)
	clone.Index = util.CloneValue(s.Index)
	if s.Variables != nil {
		clone.Variables = util.ToPtr(maps.Clone(*s.Variables))
	}
	return &clone
}

// UnmarshalJSON rejects noncanonical keys, null fields, and duplicate or
// non-string bindings. The receiver is replaced only after decoding succeeds.
func (s *OpenAPIServerConfig) UnmarshalJSON(data []byte) error {
	fields, err := decodeStrictObject(data, "OpenAPI server configuration", jsonFieldNames(reflect.TypeOf(OpenAPIServerConfig{}))...)
	if err != nil {
		return err
	}
	if raw, ok := fields["variables"]; ok {
		if err := validateOpenAPIServerBindingsJSON(raw); err != nil {
			return err
		}
	}

	// Decode the canonical struct once, without calling this method recursively.
	// New fields automatically participate instead of needing a second wire type
	// or another case in a per-field decoder.
	// Use the checked fields: decoding the original JSON could merge earlier,
	// unchecked objects when a top-level map field is repeated.
	checked, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	type plain OpenAPIServerConfig
	var decoded plain
	if err := util.DecodeJSONStrict(checked, &decoded); err != nil {
		return err
	}
	*s = OpenAPIServerConfig(decoded)
	return nil
}

// validateOpenAPIServerBindingsJSON checks a syntactically valid JSON value
// from decodeStrictObject before map decoding can erase duplicate names or turn
// null values into empty strings. Escaped names are compared after decoding.
func validateOpenAPIServerBindingsJSON(raw json.RawMessage) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token != json.Delim('{') {
		return fmt.Errorf("variables must be an object")
	}
	seen := make(map[string]bool)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		name := token.(string) // Object keys in valid JSON are always strings.
		if seen[name] {
			return fmt.Errorf("variables[%q] must not be repeated", name)
		}
		seen[name] = true
		value, err := decoder.Token()
		if err != nil {
			return err
		}
		if _, ok := value.(string); !ok {
			return fmt.Errorf("variables[%q] must be a string", name)
		}
	}
	return nil
}

// UnmarshalYAML resolves aliases and merges before applying the same strict
// JSON boundary, including indirect null fields and variable bindings.
func (s *OpenAPIServerConfig) UnmarshalYAML(node *yaml.Node) error {
	var raw common.RawJSON
	if err := raw.UnmarshalYAML(node); err != nil {
		return err
	}
	return s.UnmarshalJSON(raw)
}
