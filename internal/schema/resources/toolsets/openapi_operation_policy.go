package toolsets

import (
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/util"
	"gopkg.in/yaml.v3"
)

// OpenAPIUnsupportedOperationPolicy controls importer handling of unsupported
// operations. It never relaxes document validity, identity uniqueness, or the
// requirement to compile every explicitly included, non-excluded operation.
type OpenAPIUnsupportedOperationPolicy string

const (
	// OpenAPIUnsupportedOperationError blocks publication on any selected
	// unsupported operation. It is also the policy when the field is omitted.
	OpenAPIUnsupportedOperationError OpenAPIUnsupportedOperationPolicy = "error"
	// OpenAPIUnsupportedOperationSkip permits broad imports to skip unsupported
	// operations with inspectable diagnostics. It does not hide import failures.
	OpenAPIUnsupportedOperationSkip OpenAPIUnsupportedOperationPolicy = "skip"
)

// OpenAPISourceKeyOverride pins one operation's generated source identity. The
// importer matches its exact wire method and OpenAPI paths key before deriving
// Tool identity and canonical verbs. Matching and final identity collision
// checks require the document; authoring validation never parses it here.
type OpenAPISourceKeyOverride struct {
	// Method uses the case sent on the wire: GET for the fixed get field, or
	// the exact additionalOperations key for an OpenAPI 3.2 extension method.
	Method string `json:"method" yaml:"method"`
	// Path is the exact paths key, including parameter names such as /items/{id}.
	// It is not a full URL or a rendered path; no cleaning or decoding is applied.
	Path string `json:"path" yaml:"path"`
	// SourceKey replaces operationId or the method/path fallback as the stable
	// generated identity. It does not rename the upstream operation or filters.
	SourceKey string `json:"sourceKey" yaml:"sourceKey"`
}

// Validate checks method token syntax, a leading path slash, and a nonblank
// identity while preserving every authored string. Full path-template syntax,
// operation membership, and compiler support are importer responsibilities.
func (o *OpenAPISourceKeyOverride) Validate(vc *common.ValidationContext) error {
	vc = validationContext(vc)
	if o == nil {
		return vc.NewError("OpenAPI source key override is required")
	}

	var result *multierror.Error
	if !tools.IsHTTPToken(o.Method) {
		result = multierror.Append(result, vc.NewErrorForField("method", "must be an HTTP token"))
	}
	if !strings.HasPrefix(o.Path, "/") {
		result = multierror.Append(result, vc.NewErrorForField("path", "must start with /"))
	}
	if strings.TrimSpace(o.SourceKey) == "" {
		result = multierror.Append(result, vc.NewErrorForField("sourceKey", "must not be blank"))
	}
	return result.ErrorOrNil()
}

// validateOpenAPISourceKeyOverrides rejects ambiguous targets and identities
// within the authored list. It compares exact strings without normalizing HTTP
// methods or paths. Collisions with unoverridden operations are importer checks.
func validateOpenAPISourceKeyOverrides(overrides []OpenAPISourceKeyOverride, vc *common.ValidationContext) error {
	var result *multierror.Error
	targets := make(map[[2]string]bool, len(overrides))
	keys := make(map[string]bool, len(overrides))
	for i := range overrides {
		override := &overrides[i]
		path := vc.PushIndex(i)
		result = multierror.Append(result, override.Validate(path))
		target := [2]string{override.Method, override.Path}
		if targets[target] {
			result = multierror.Append(result, path.NewError("method and path must not duplicate another override"))
		}
		if keys[override.SourceKey] {
			result = multierror.Append(result, path.NewErrorForField("sourceKey", "must not duplicate another override"))
		}
		targets[target] = true
		keys[override.SourceKey] = true
	}
	return result.ErrorOrNil()
}

// UnmarshalJSON rejects noncanonical, unknown, and null fields before ordinary
// string decoding can erase explicit nulls. Missing fields remain visible to
// Validate, and failed decoding preserves the receiver.
func (o *OpenAPISourceKeyOverride) UnmarshalJSON(data []byte) error {
	if _, err := decodeStrictObject(data, "OpenAPI source key override", "method", "path", "sourceKey"); err != nil {
		return err
	}
	type plain OpenAPISourceKeyOverride
	var decoded plain
	if err := util.DecodeJSONStrict(data, &decoded); err != nil {
		return err
	}
	*o = OpenAPISourceKeyOverride(decoded)
	return nil
}

// UnmarshalYAML resolves aliases and merges before applying strict JSON rules.
func (o *OpenAPISourceKeyOverride) UnmarshalYAML(node *yaml.Node) error {
	var raw common.RawJSON
	if err := raw.UnmarshalYAML(node); err != nil {
		return err
	}
	return o.UnmarshalJSON(raw)
}
