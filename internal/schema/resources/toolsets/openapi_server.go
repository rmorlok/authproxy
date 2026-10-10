package toolsets

import (
	"fmt"
	"maps"
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
		if s.Index == nil || s.URL != nil {
			result = multierror.Append(result, vc.NewErrorForField("variables", "is only supported with index, not a URL override"))
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

// UnmarshalJSON rejects noncanonical keys, null fields, and null bindings before
// decoding can mistake them for omitted settings or empty string values. The
// receiver is replaced only after all fields have decoded successfully.
func (s *OpenAPIServerConfig) UnmarshalJSON(data []byte) error {
	fields, err := decodeStrictObject(data, "OpenAPI server configuration", "url", "index", "variables")
	if err != nil {
		return err
	}
	var decoded OpenAPIServerConfig
	for field, raw := range fields {
		switch field {
		case "url":
			if err := util.DecodeJSONStrict(raw, &decoded.URL); err != nil {
				return fmt.Errorf("decode url: %w", err)
			}
		case "index":
			if err := util.DecodeJSONStrict(raw, &decoded.Index); err != nil {
				return fmt.Errorf("decode index: %w", err)
			}
		case "variables":
			var bindings map[string]*string
			if err := util.DecodeJSONStrict(raw, &bindings); err != nil {
				return fmt.Errorf("decode variables: %w", err)
			}
			values := make(map[string]string, len(bindings))
			for name, value := range bindings {
				if value == nil {
					return fmt.Errorf("variables[%q] must be a string, not null", name)
				}
				values[name] = *value
			}
			decoded.Variables = &values
		}
	}
	*s = decoded
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
