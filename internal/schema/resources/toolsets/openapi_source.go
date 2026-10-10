package toolsets

import (
	"encoding/json"
	"slices"

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
	Server     *OpenAPIServerConfig    `json:"server,omitempty" yaml:"server,omitempty"`
}

// OpenAPIOperationFilter selects exact document operationId values. Omitted
// inclusion is unrestricted, including operations without IDs; a supplied
// inclusion must be nonempty and only matches named operations. Exclusions win
// overlap. These IDs are not normalized or interpreted as fallback source keys.
type OpenAPIOperationFilter struct {
	IncludeOperationIDs []string `json:"includeOperationIds,omitempty" yaml:"includeOperationIds,omitempty"`
	ExcludeOperationIDs []string `json:"excludeOperationIds,omitempty" yaml:"excludeOperationIds,omitempty"`
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

	return f.lists().validate(vc)
}

// lists views the filter through the shared include/exclude name rules.
func (f OpenAPIOperationFilter) lists() exactNameLists {
	return exactNameLists{
		includeField: "includeOperationIds",
		include:      f.IncludeOperationIDs,
		excludeField: "excludeOperationIds",
		exclude:      f.ExcludeOperationIDs,
	}
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

// UnmarshalJSON rejects noncanonical fields and null objects before ordinary
// pointer decoding could erase their presence. Failed decoding is atomic.
func (s *OpenAPISource) UnmarshalJSON(data []byte) error {
	if _, err := decodeStrictObject(data, "OpenAPI source", "document", "operations", "server"); err != nil {
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
	lists := OpenAPIOperationFilter{}.lists()
	if err := lists.decode(data, "OpenAPI operation filter"); err != nil {
		return err
	}

	*f = OpenAPIOperationFilter{IncludeOperationIDs: lists.include, ExcludeOperationIDs: lists.exclude}

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

// MarshalJSON retains explicit empty lists, especially invalid inclusion that
// must not turn into an omitted, unrestricted import when serialized.
func (f OpenAPIOperationFilter) MarshalJSON() ([]byte, error) {
	return json.Marshal(f.lists().fieldsForMarshal())
}

// MarshalYAML preserves the same list presence as JSON using detached slices.
func (f OpenAPIOperationFilter) MarshalYAML() (any, error) {
	return f.lists().fieldsForMarshal(), nil
}
