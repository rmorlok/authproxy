package toolsets

import (
	"regexp"
	"slices"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util"
)

// SourceDiagnosticSeverity describes the impact of one import finding, not the
// readiness of a snapshot or catalog. Publication still requires compilation.
type SourceDiagnosticSeverity string

const (
	SourceDiagnosticSeverityWarning SourceDiagnosticSeverity = "warning"
	SourceDiagnosticSeverityError   SourceDiagnosticSeverity = "error"
)

// SourceDiagnostic is a server-produced explanation of an import finding.
// OpenAPI generation reports and connection-specific MCP catalog reports can
// share this value without sharing their observations or source context.
// Producers must use safe messages and locations, excluding credentials, raw
// upstream error bodies, and connection configuration values.
type SourceDiagnostic struct {
	Severity SourceDiagnosticSeverity `json:"severity" yaml:"severity"`

	// Code is an extensible machine-readable reason. Consumers must tolerate
	// new codes and use Message for the human-readable explanation.
	Code    string `json:"code" yaml:"code"`
	Message string `json:"message" yaml:"message"`

	// SourceKey is the exact operation/tool identity, when one is known. Parse,
	// acquisition, and identity failures can occur before a key is available.
	SourceKey string                    `json:"sourceKey,omitempty" yaml:"sourceKey,omitempty"`
	Location  *SourceDiagnosticLocation `json:"location,omitempty" yaml:"location,omitempty"`
}

// SourceDiagnosticLocation identifies a document or a value within it. It is
// descriptive provenance, never permission to fetch or evaluate remote content.
type SourceDiagnosticLocation struct {
	// DocumentURI is an optional absolute, fragment-free document identity.
	// Producers must redact private query parameters before exposing a URI;
	// syntactic validation cannot establish whether an address contains secrets.
	// Omission refers to the containing report's source document/catalog.
	DocumentURI string `json:"documentUri,omitempty" yaml:"documentUri,omitempty"`

	// JSONPointer uses RFC 6901 string syntax, not URI-fragment syntax. A
	// supplied empty string identifies the root; nil means no specific value.
	JSONPointer *string `json:"jsonPointer,omitempty" yaml:"jsonPointer,omitempty"`
}

// sourceJSONPointerPattern checks syntax only. Tokens may contain slashes as
// separators and encode literal tildes/slashes as ~0/~1. Resolving a pointer
// against the actual source remains the diagnostic producer's responsibility.
var sourceJSONPointerPattern = regexp.MustCompile(`^(?:/(?:[^~]|~[01])*)*$`)

// Validate checks a diagnostic's vocabulary and location without resolving its
// source key or inferring whether a retained snapshot is publishable.
func (d *SourceDiagnostic) Validate(vc *common.ValidationContext) error {
	vc = validationContext(vc)

	if d == nil {
		return vc.NewError("source diagnostic is required")
	}

	var result *multierror.Error

	switch d.Severity {
	case SourceDiagnosticSeverityWarning, SourceDiagnosticSeverityError:
	default:
		result = multierror.Append(result, vc.NewErrorForField("severity", "must be warning or error"))
	}

	if strings.TrimSpace(d.Code) == "" {
		result = multierror.Append(result, vc.NewErrorForField("code", "must not be blank"))
	}

	if strings.TrimSpace(d.Message) == "" {
		result = multierror.Append(result, vc.NewErrorForField("message", "must not be blank"))
	}

	if d.SourceKey != "" && strings.TrimSpace(d.SourceKey) == "" {
		result = multierror.Append(result, vc.NewErrorForField("sourceKey", "must not be blank when supplied"))
	}

	result = multierror.Append(result, d.Location.Validate(vc.PushField("location")))

	return result.ErrorOrNil()
}

// Validate accepts an omitted location and otherwise requires a document URI
// or pointer. It checks URI/pointer syntax without fetching source content.
func (l *SourceDiagnosticLocation) Validate(vc *common.ValidationContext) error {
	if l == nil {
		return nil
	}

	vc = validationContext(vc)
	var result *multierror.Error

	if l.DocumentURI == "" && l.JSONPointer == nil {
		result = multierror.Append(result, vc.NewError("must identify a document URI or JSON pointer"))
	}

	if l.DocumentURI != "" {
		if err := validateOpenAPIReferenceURI(l.DocumentURI); err != nil {
			result = multierror.Append(result, vc.NewErrorForField("documentUri", err.Error()))
		}
	}

	if l.JSONPointer != nil && !sourceJSONPointerPattern.MatchString(*l.JSONPointer) {
		result = multierror.Append(result, vc.NewErrorForField("jsonPointer", "must use JSON Pointer string syntax"))
	}

	return result.ErrorOrNil()
}

// Clone detaches optional location data, including an explicitly empty root
// pointer. Nil and invalid values are retained for later validation.
func (d *SourceDiagnostic) Clone() *SourceDiagnostic {
	if d == nil {
		return nil
	}

	clone := *d
	clone.Location = util.CloneValue(d.Location)

	if clone.Location != nil {
		clone.Location.JSONPointer = util.CloneValue(d.Location.JSONPointer)
	}

	return &clone
}

// Clone detaches generation status and its diagnostic locations so a candidate
// or invalid authored patch cannot mutate another generation's observations.
func (s *ToolSetStatus) Clone() *ToolSetStatus {
	if s == nil {
		return nil
	}

	clone := *s
	clone.ImportDiagnostics = slices.Clone(s.ImportDiagnostics)

	for i := range clone.ImportDiagnostics {
		clone.ImportDiagnostics[i] = *s.ImportDiagnostics[i].Clone()
	}

	return &clone
}
