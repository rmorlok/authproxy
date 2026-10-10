package toolsets

import (
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util"
	"gopkg.in/yaml.v3"
)

// MCPTransport identifies the remote transport requested by an authored source.
// Protocol-version negotiation and supported capabilities belong to the adapter.
type MCPTransport string

// MCPTransportStreamableHTTP is the supported remote HTTP transport contract.
const MCPTransportStreamableHTTP MCPTransport = "streamableHttp"

// MCPSource declares how each bound connection discovers a remote tool catalog.
// The generation pins these settings, not the provider's changing inventory.
// Contract validation performs no rendering, discovery, or network requests.
type MCPSource struct {
	// Endpoint is an exact endpoint template, which may refer to connection cfg.
	// URL and template compilation occur later, before authenticated requests.
	Endpoint  string       `json:"endpoint" yaml:"endpoint"`
	Transport MCPTransport `json:"transport" yaml:"transport"`
	// RefreshInterval is optional positive Go duration text, preserved verbatim.
	// A string avoids common.HumanDuration's fractional serialization mismatch;
	// omission leaves runtime policy to choose a refresh interval.
	RefreshInterval *string        `json:"refreshInterval,omitempty" yaml:"refreshInterval,omitempty"`
	Tools           *MCPToolFilter `json:"tools,omitempty" yaml:"tools,omitempty"`
}

// MCPToolFilter selects exact upstream names without changing their routing
// identity. Omitted IncludeNames includes all names; a supplied empty list is
// invalid. Exclusions take precedence, including when a name is also included.
type MCPToolFilter struct {
	IncludeNames []string `json:"includeNames,omitempty" yaml:"includeNames,omitempty"`
	ExcludeNames []string `json:"excludeNames,omitempty" yaml:"excludeNames,omitempty"`
}

// Validate checks authored source shape without interpreting endpoint templates
// or choosing an interval default. Unsupported transport names fail explicitly.
func (s *MCPSource) Validate(vc *common.ValidationContext) error {
	vc = validationContext(vc)

	if s == nil {
		return vc.NewError("MCP source is required")
	}

	var result *multierror.Error
	if strings.TrimSpace(s.Endpoint) == "" {
		result = multierror.Append(result, vc.NewErrorForField("endpoint", "must not be blank"))
	} else if strings.TrimSpace(s.Endpoint) != s.Endpoint || strings.ContainsAny(s.Endpoint, "\r\n") {
		result = multierror.Append(result, vc.NewErrorForField("endpoint", "must not contain surrounding whitespace or newlines"))
	}

	if s.Transport != MCPTransportStreamableHTTP {
		result = multierror.Append(result, vc.NewErrorForField("transport", "must be streamableHttp"))
	}

	if s.RefreshInterval != nil {
		interval, err := time.ParseDuration(*s.RefreshInterval)
		if err != nil || interval <= 0 {
			result = multierror.Append(result, vc.NewErrorForField("refreshInterval", "must be a positive Go duration"))
		}
	}

	result = multierror.Append(result, s.Tools.Validate(vc.PushField("tools")))

	return result.ErrorOrNil()
}

// Validate checks nonblank, unique names within each list. Names are never
// trimmed or normalized; overlap across lists is valid because exclusion wins.
// A nil filter imposes no name restrictions.
func (f *MCPToolFilter) Validate(vc *common.ValidationContext) error {
	if f == nil {
		return nil
	}

	return f.lists().validate(vc)
}

// lists views the filter through the shared include/exclude name rules.
func (f MCPToolFilter) lists() exactNameLists {
	return exactNameLists{
		includeField: "includeNames",
		include:      f.IncludeNames,
		excludeField: "excludeNames",
		exclude:      f.ExcludeNames,
	}
}

// Clone returns a detached source, preserving omitted fields and authored text.
func (s *MCPSource) Clone() *MCPSource {
	if s == nil {
		return nil
	}
	clone := *s
	clone.RefreshInterval = util.CloneValue(s.RefreshInterval)
	clone.Tools = s.Tools.Clone()
	return &clone
}

// Clone preserves nil versus empty lists while detaching their backing arrays.
func (f *MCPToolFilter) Clone() *MCPToolFilter {
	if f == nil {
		return nil
	}
	return &MCPToolFilter{
		IncludeNames: slices.Clone(f.IncludeNames),
		ExcludeNames: slices.Clone(f.ExcludeNames),
	}
}

// UnmarshalJSON rejects unknown, mis-cased, and null configuration fields before
// ordinary decoding could mistake a null optional value for omission.
func (s *MCPSource) UnmarshalJSON(data []byte) error {
	if _, err := decodeStrictObject(data, "MCP configuration", "endpoint", "transport", "refreshInterval", "tools"); err != nil {
		return err
	}
	type plain MCPSource
	var decoded plain
	if err := util.DecodeJSONStrict(data, &decoded); err != nil {
		return err
	}
	*s = MCPSource(decoded)
	return nil
}

// UnmarshalYAML resolves YAML aliases and merges before the strict JSON boundary
// so null settings and malformed string values retain their original meaning.
func (s *MCPSource) UnmarshalYAML(node *yaml.Node) error {
	var raw common.RawJSON
	if err := raw.UnmarshalYAML(node); err != nil {
		return err
	}
	return s.UnmarshalJSON(raw)
}

// UnmarshalJSON retains list presence and rejects null list elements rather
// than turning them into empty names. Validation handles blank and duplicate names.
func (f *MCPToolFilter) UnmarshalJSON(data []byte) error {
	lists := MCPToolFilter{}.lists()
	if err := lists.decode(data, "MCP configuration"); err != nil {
		return err
	}

	*f = MCPToolFilter{IncludeNames: lists.include, ExcludeNames: lists.exclude}

	return nil
}

// UnmarshalYAML shares strict list handling with JSON, including aliased nulls.
func (f *MCPToolFilter) UnmarshalYAML(node *yaml.Node) error {
	var raw common.RawJSON
	if err := raw.UnmarshalYAML(node); err != nil {
		return err
	}
	return f.UnmarshalJSON(raw)
}

// MarshalJSON retains explicitly empty lists, especially invalid includeNames:[]
// which must never serialize as an omitted, unrestricted inclusion policy.
func (f MCPToolFilter) MarshalJSON() ([]byte, error) {
	return json.Marshal(f.lists().fieldsForMarshal())
}

// MarshalYAML preserves the same list presence as JSON without exposing aliases.
func (f MCPToolFilter) MarshalYAML() (any, error) {
	return f.lists().fieldsForMarshal(), nil
}
