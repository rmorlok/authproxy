package toolsets

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util"
	"gopkg.in/yaml.v3"
)

// OpenAPISource declares a document to acquire and import for a generation.
// Validation checks authored shape only; acquisition, operation discovery,
// security resolution, compilation, and snapshot publication happen separately.
type OpenAPISource struct {
	Document   *OpenAPIDocument        `json:"document" yaml:"document"`
	Operations *OpenAPIOperationFilter `json:"operations,omitempty" yaml:"operations,omitempty"`
	Server     *OpenAPIServerOverride  `json:"server,omitempty" yaml:"server,omitempty"`
}

// OpenAPIOperationFilter selects exact document operationId values. Omitted
// inclusion is unrestricted, including operations without IDs; a supplied
// inclusion must be nonempty and only matches named operations. Exclusions win
// overlap. These IDs are not normalized or interpreted as fallback source keys.
type OpenAPIOperationFilter struct {
	IncludeOperationIDs []string `json:"includeOperationIds,omitempty" yaml:"includeOperationIds,omitempty"`
	ExcludeOperationIDs []string `json:"excludeOperationIds,omitempty" yaml:"excludeOperationIds,omitempty"`
}

// OpenAPIServerOverride deliberately replaces the document-selected server base
// URL. Omission preserves the importer's server precedence and relative-URL
// rules; authoring this override does not select a server index or bind variables.
type OpenAPIServerOverride struct {
	// URL may contain connection cfg templates, including a whole-URL template.
	// Template compilation and rendered URL validation precede later execution.
	URL string `json:"url" yaml:"url"`
}

// Validate checks the required document and optional import settings without
// fetching content, parsing an OpenAPI specification, or rendering templates.
func (s *OpenAPISource) Validate(vc *common.ValidationContext) error {
	vc = validationContext(vc)
	if s == nil {
		return vc.NewError("OpenAPI source is required")
	}
	var result *multierror.Error
	result = multierror.Append(result, s.Document.Validate(vc.PushField("document")))
	result = multierror.Append(result, s.Operations.Validate(vc.PushField("operations")))
	result = multierror.Append(result, s.Server.Validate(vc.PushField("server")))
	return result.ErrorOrNil()
}

// Validate checks nonblank, per-list unique IDs while preserving authored text.
// A nil filter or omitted inclusion imposes no inclusion restriction. Catalog
// membership and duplicate operation IDs in the document are importer checks.
func (f *OpenAPIOperationFilter) Validate(vc *common.ValidationContext) error {
	if f == nil {
		return nil
	}
	vc = validationContext(vc)
	var result *multierror.Error
	if f.IncludeOperationIDs != nil && len(f.IncludeOperationIDs) == 0 {
		result = multierror.Append(result, vc.NewErrorForField("includeOperationIds", "must not be empty when supplied"))
	}
	for _, list := range []struct {
		field string
		ids   []string
	}{{"includeOperationIds", f.IncludeOperationIDs}, {"excludeOperationIds", f.ExcludeOperationIDs}} {
		seen := make(map[string]bool, len(list.ids))
		for i, id := range list.ids {
			path := vc.PushField(list.field).PushIndex(i)
			if strings.TrimSpace(id) == "" {
				result = multierror.Append(result, path.NewError("must not be blank"))
			}
			if seen[id] {
				result = multierror.Append(result, path.NewError("must be unique within the list"))
			}
			seen[id] = true
		}
	}
	return result.ErrorOrNil()
}

// Validate requires a nonblank override without surrounding whitespace or line
// breaks. A nil override is valid; template and URL semantics are checked later.
func (s *OpenAPIServerOverride) Validate(vc *common.ValidationContext) error {
	if s == nil {
		return nil
	}
	vc = validationContext(vc)
	if strings.TrimSpace(s.URL) == "" {
		return vc.NewErrorForField("url", "must not be blank")
	}
	if strings.TrimSpace(s.URL) != s.URL || strings.ContainsAny(s.URL, "\r\n") {
		return vc.NewErrorForField("url", "must not contain surrounding whitespace or newlines")
	}
	return nil
}

// Clone returns a detached source, including document data and optional import
// settings. It preserves absent or invalid fields for later diagnostics.
func (s *OpenAPISource) Clone() *OpenAPISource {
	if s == nil {
		return nil
	}
	clone := *s
	clone.Document = s.Document.Clone()
	clone.Operations = s.Operations.Clone()
	clone.Server = s.Server.Clone()
	return &clone
}

// Clone detaches both operation lists while retaining nil versus explicit [].
func (f *OpenAPIOperationFilter) Clone() *OpenAPIOperationFilter {
	if f == nil {
		return nil
	}
	return &OpenAPIOperationFilter{
		IncludeOperationIDs: slices.Clone(f.IncludeOperationIDs),
		ExcludeOperationIDs: slices.Clone(f.ExcludeOperationIDs),
	}
}

// Clone returns an independent optional server override without normalization.
func (s *OpenAPIServerOverride) Clone() *OpenAPIServerOverride {
	return util.CloneValue(s)
}

// UnmarshalJSON rejects noncanonical fields and null objects before ordinary
// pointer decoding could erase their presence. Failed decoding is atomic.
func (s *OpenAPISource) UnmarshalJSON(data []byte) error {
	if _, err := decodeOpenAPISourceObject(data, "document", "operations", "server"); err != nil {
		return err
	}
	type plain OpenAPISource
	var decoded plain
	if err := util.DecodeJSONStrict(data, &decoded); err != nil {
		return err
	}
	*s = OpenAPISource(decoded)
	return nil
}

// UnmarshalJSON retains list presence and rejects null entries instead of
// turning them into empty operation IDs. Validate checks names and duplicates.
func (f *OpenAPIOperationFilter) UnmarshalJSON(data []byte) error {
	fields, err := decodeOpenAPISourceObject(data, "includeOperationIds", "excludeOperationIds")
	if err != nil {
		return err
	}
	var decoded OpenAPIOperationFilter
	for field, raw := range fields {
		var ids []*string
		if err := util.DecodeJSONStrict(raw, &ids); err != nil {
			return fmt.Errorf("decode %s: %w", field, err)
		}
		values := make([]string, len(ids))
		for i, id := range ids {
			if id == nil {
				return fmt.Errorf("%s[%d] must be a string, not null", field, i)
			}
			values[i] = *id
		}
		if field == "includeOperationIds" {
			decoded.IncludeOperationIDs = values
		} else {
			decoded.ExcludeOperationIDs = values
		}
	}
	*f = decoded
	return nil
}

// UnmarshalJSON accepts only the canonical non-null url field. Its required
// value is checked by Validate, and failed decoding preserves the receiver.
func (s *OpenAPIServerOverride) UnmarshalJSON(data []byte) error {
	if _, err := decodeOpenAPISourceObject(data, "url"); err != nil {
		return err
	}
	type plain OpenAPIServerOverride
	var decoded plain
	if err := util.DecodeJSONStrict(data, &decoded); err != nil {
		return err
	}
	*s = OpenAPIServerOverride(decoded)
	return nil
}

// UnmarshalYAML resolves aliases and merges before applying strict source rules.
func (s *OpenAPISource) UnmarshalYAML(node *yaml.Node) error {
	var raw common.RawJSON
	if err := raw.UnmarshalYAML(node); err != nil {
		return err
	}
	return s.UnmarshalJSON(raw)
}

// UnmarshalYAML retains list nulls introduced through YAML aliases or merges.
func (f *OpenAPIOperationFilter) UnmarshalYAML(node *yaml.Node) error {
	var raw common.RawJSON
	if err := raw.UnmarshalYAML(node); err != nil {
		return err
	}
	return f.UnmarshalJSON(raw)
}

// UnmarshalYAML resolves composition before validating the owned url field.
func (s *OpenAPIServerOverride) UnmarshalYAML(node *yaml.Node) error {
	var raw common.RawJSON
	if err := raw.UnmarshalYAML(node); err != nil {
		return err
	}
	return s.UnmarshalJSON(raw)
}

// MarshalJSON retains explicit empty lists, especially invalid inclusion that
// must not turn into an omitted, unrestricted import when serialized.
func (f OpenAPIOperationFilter) MarshalJSON() ([]byte, error) {
	return json.Marshal(f.fieldsForMarshal())
}

// MarshalYAML preserves the same list presence as JSON using detached slices.
func (f OpenAPIOperationFilter) MarshalYAML() (any, error) {
	return f.fieldsForMarshal(), nil
}

// fieldsForMarshal includes only supplied lists and retains non-nil empties.
func (f OpenAPIOperationFilter) fieldsForMarshal() map[string][]string {
	fields := make(map[string][]string)
	if f.IncludeOperationIDs != nil {
		fields["includeOperationIds"] = slices.Clone(f.IncludeOperationIDs)
	}
	if f.ExcludeOperationIDs != nil {
		fields["excludeOperationIds"] = slices.Clone(f.ExcludeOperationIDs)
	}
	return fields
}

// decodeOpenAPISourceObject checks owned canonical keys and explicit nulls
// before pointer decoding. Complete-value validation checks required fields.
func decodeOpenAPISourceObject(data []byte, allowed ...string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := util.DecodeJSONStrict(data, &fields); err != nil {
		return nil, err
	}
	if fields == nil {
		return nil, fmt.Errorf("OpenAPI source settings must be an object")
	}
	for field, raw := range fields {
		if !slices.Contains(allowed, field) {
			return nil, fmt.Errorf("unknown OpenAPI source setting %q", field)
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil, fmt.Errorf("%s must not be null", field)
		}
	}
	return fields, nil
}
