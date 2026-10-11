package toolsets

import (
	"encoding/json"
	"reflect"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/util"
	"gopkg.in/yaml.v3"
)

// OpenAPIReferenceFetch declares one external document to acquire and snapshot
// during import. It permits only this URL, not an origin or recursive traversal.
// The importer must check acquisition authority, enforce bounds, and reject
// redirects; validators and tool invocations never perform these fetches.
type OpenAPIReferenceFetch struct {
	// URL is the literal HTTP(S) retrieval address and document identity. It
	// excludes fragments; a reference within the source may select a fragment
	// from the acquired document. Relative references use this retrieval context.
	URL string `json:"url" yaml:"url"`

	// FetchConnectionRef authenticates only this entry. Omission means an
	// unauthenticated fetch, even when document or another fetch has credentials.
	// The importer resolves and authorizes this connection separately from the
	// target connection receiving tools; credentials never enter the snapshot.
	FetchConnectionRef *meta.ObjectReference `json:"fetchConnectionRef,omitempty" yaml:"fetchConnectionRef,omitempty"`
}

// Validate checks a literal acquisition address and optional connection identity
// without I/O. Cross-entry identity collisions are checked by the containing
// reference configuration and, for normalized identities, the importer.
func (f *OpenAPIReferenceFetch) Validate(vc *common.ValidationContext) error {
	vc = validationContext(vc)
	if f == nil {
		return vc.NewError("OpenAPI reference fetch is required")
	}

	var result *multierror.Error
	if err := validateOpenAPIDocumentURL(f.URL); err != nil {
		result = multierror.Append(result, vc.NewErrorForField("url", err.Error()))
	}

	if f.FetchConnectionRef != nil {
		result = multierror.Append(result, tools.ValidateConnectionReference(
			*f.FetchConnectionRef, "", vc.PushField("fetchConnectionRef"),
		))
	}

	return result.ErrorOrNil()
}

// UnmarshalJSON accepts canonical, non-null acquisition fields. It checks the
// connection reference before typed decoding can discard forbidden generation
// or null fields. Failed decoding leaves the receiver unchanged.
func (f *OpenAPIReferenceFetch) UnmarshalJSON(data []byte) error {
	fields, err := decodeStrictObject(
		data,
		"OpenAPI reference fetch",
		jsonFieldNames(reflect.TypeOf(OpenAPIReferenceFetch{}))...,
	)
	if err != nil {
		return err
	}

	if raw, ok := fields["fetchConnectionRef"]; ok {
		if err := validateOpenAPIFetchReferenceFields(raw); err != nil {
			return err
		}
	}

	// Decode only checked fields so repeated reference fields cannot merge
	// earlier unchecked connection settings into the final entry.
	checked, err := json.Marshal(fields)
	if err != nil {
		return err
	}

	type plain OpenAPIReferenceFetch
	var decoded plain
	if err := util.DecodeJSONStrict(checked, &decoded); err != nil {
		return err
	}

	*f = OpenAPIReferenceFetch(decoded)

	return nil
}

// UnmarshalYAML resolves aliases and merges through the same strict JSON rules.
func (f *OpenAPIReferenceFetch) UnmarshalYAML(node *yaml.Node) error {
	var raw common.RawJSON
	if err := raw.UnmarshalYAML(node); err != nil {
		return err
	}
	return f.UnmarshalJSON(raw)
}
