package toolsets

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util"
	"gopkg.in/yaml.v3"
)

// PermissionMapping assigns administrator-authored permission aliases to
// imported tools selected by stable source key. All matching mappings are
// additive: their aliases supplement, never replace, the tool's canonical verb.
// These contracts do not resolve aliases or grant permission by themselves.
type PermissionMapping struct {
	Match    SourceKeyMatch `json:"match" yaml:"match"`
	AddVerbs []string       `json:"addVerbs" yaml:"addVerbs"`
}

// SourceKeyMatch selects stable imported identities by exact key or pattern.
// The lists combine with OR and must contain at least one entry in total. Keys
// remain unnormalized; patterns use Go regular expressions matched against the
// entire key, including slash characters in an OpenAPI method/path fallback.
// Broad patterns deliberately include matching tools introduced in the future.
type SourceKeyMatch struct {
	SourceKeys        []string `json:"sourceKeys,omitempty" yaml:"sourceKeys,omitempty"`
	SourceKeyPatterns []string `json:"sourceKeyPatterns,omitempty" yaml:"sourceKeyPatterns,omitempty"`
}

// Validate checks matcher syntax and nonempty, unique added aliases without
// consulting a source catalog. It preserves exact keys, patterns, and verb order.
func (p *PermissionMapping) Validate(vc *common.ValidationContext) error {
	vc = validationContext(vc)

	if p == nil {
		return vc.NewError("permission mapping is required")
	}

	var result *multierror.Error
	result = multierror.Append(result, p.Match.Validate(vc.PushField("match")))

	if len(p.AddVerbs) == 0 {
		result = multierror.Append(result, vc.NewErrorForField("addVerbs", "must contain at least one permission verb"))
	}

	result = multierror.Append(result, validateMappingStrings(p.AddVerbs, true, vc.PushField("addVerbs")))

	return result.ErrorOrNil()
}

// Validate requires at least one nonblank exact key or syntactically valid
// full-string Go regular expression. Duplicates within either list are invalid;
// overlap between exact keys and patterns is allowed because matching is OR.
func (m *SourceKeyMatch) Validate(vc *common.ValidationContext) error {
	vc = validationContext(vc)

	if m == nil {
		return vc.NewError("source key match is required")
	}

	var result *multierror.Error
	if len(m.SourceKeys)+len(m.SourceKeyPatterns) == 0 {
		result = multierror.Append(result, vc.NewError("must contain at least one source key or source key pattern"))
	}

	result = multierror.Append(result, validateMappingStrings(m.SourceKeys, false, vc.PushField("sourceKeys")))
	result = multierror.Append(result, validateMappingStrings(m.SourceKeyPatterns, false, vc.PushField("sourceKeyPatterns")))

	for i, pattern := range m.SourceKeyPatterns {
		// Validate the pattern independently before wrapping it. Otherwise an
		// unmatched closing/opening pair could escape the enclosing group and
		// yield a valid expression whose alternatives bypass an absolute anchor.
		if _, err := regexp.Compile(pattern); err != nil {
			result = multierror.Append(result, vc.PushField("sourceKeyPatterns").PushIndex(i).NewErrorf("must be a valid Go regular expression: %v", err))
			continue
		}

		// Absolute anchors retain whole-key semantics even with inline (?m).
		if _, err := regexp.Compile(`\A(?:` + pattern + `)\z`); err != nil {
			result = multierror.Append(result, vc.PushField("sourceKeyPatterns").PushIndex(i).NewErrorf("must be a valid full-string Go regular expression: %v", err))
		}
	}
	return result.ErrorOrNil()
}

// validateMappingStrings rejects blank and duplicate entries. Verbs additionally
// reject surrounding whitespace; exact source keys and patterns retain it.
func validateMappingStrings(values []string, requireTrimmed bool, vc *common.ValidationContext) error {
	var result *multierror.Error
	seen := make(map[string]bool, len(values))

	for i, value := range values {
		path := vc.PushIndex(i)

		if strings.TrimSpace(value) == "" {
			result = multierror.Append(result, path.NewError("must not be blank"))
		} else if requireTrimmed && strings.TrimSpace(value) != value {
			result = multierror.Append(result, path.NewError("must not have surrounding whitespace"))
		}

		if seen[value] {
			result = multierror.Append(result, path.NewError("must not duplicate another entry"))
		}
		seen[value] = true
	}

	return result.ErrorOrNil()
}

// Clone returns a detached mapping, preserving missing and explicit empty lists
// without serializing or losing values needed for validation diagnostics.
func (p *PermissionMapping) Clone() *PermissionMapping {
	if p == nil {
		return nil
	}
	clone := *p
	clone.Match = *p.Match.Clone()
	clone.AddVerbs = slices.Clone(p.AddVerbs)
	return &clone
}

// Clone returns an independent matcher with the original list presence/order.
func (m *SourceKeyMatch) Clone() *SourceKeyMatch {
	if m == nil {
		return nil
	}
	clone := *m
	clone.SourceKeys = slices.Clone(m.SourceKeys)
	clone.SourceKeyPatterns = slices.Clone(m.SourceKeyPatterns)
	return &clone
}

// UnmarshalJSON rejects unknown or noncanonical names and explicit null values.
// Missing fields remain visible to Validate; failed decoding is atomic.
func (p *PermissionMapping) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := util.DecodeJSONStrict(data, &fields); err != nil {
		return err
	}

	if fields == nil {
		return fmt.Errorf("permission mapping must be an object")
	}

	var decoded PermissionMapping
	for name, raw := range fields {
		switch name {
		case "match":
			if err := util.DecodeJSONStrict(raw, &decoded.Match); err != nil {
				return fmt.Errorf("decode match: %w", err)
			}
		case "addVerbs":
			values, err := decodeMappingStrings(raw, name)
			if err != nil {
				return err
			}
			decoded.AddVerbs = values
		default:
			return fmt.Errorf("unknown permission mapping field %q", name)
		}
	}

	*p = decoded

	return nil
}

// UnmarshalJSON accepts exact matcher field names and string collections only,
// rejecting null collections or elements even when another matcher is valid.
func (m *SourceKeyMatch) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := util.DecodeJSONStrict(data, &fields); err != nil {
		return err
	}

	if fields == nil {
		return fmt.Errorf("source key match must be an object")
	}

	var decoded SourceKeyMatch

	for name, raw := range fields {
		if name != "sourceKeys" && name != "sourceKeyPatterns" {
			return fmt.Errorf("unknown source key match field %q", name)
		}

		values, err := decodeMappingStrings(raw, name)
		if err != nil {
			return err
		}

		if name == "sourceKeys" {
			decoded.SourceKeys = values
		} else {
			decoded.SourceKeyPatterns = values
		}
	}
	*m = decoded
	return nil
}

// decodeMappingStrings retains an explicit empty array but rejects null arrays
// and entries before ordinary string decoding could turn null into an empty key.
func decodeMappingStrings(data []byte, field string) ([]string, error) {
	var values []*string
	if err := util.DecodeJSONStrict(data, &values); err != nil {
		return nil, fmt.Errorf("decode %s: %w", field, err)
	}

	if values == nil {
		return nil, fmt.Errorf("%s must be an array, not null", field)
	}

	result := make([]string, len(values))
	for i, value := range values {
		if value == nil {
			return nil, fmt.Errorf("%s[%d] must be a string, not null", field, i)
		}
		result[i] = *value
	}

	return result, nil
}

// UnmarshalYAML resolves aliases and merge keys before strict JSON decoding so
// null matches and collections cannot silently become omitted fields.
func (p *PermissionMapping) UnmarshalYAML(node *yaml.Node) error {
	var raw common.RawJSON
	if err := raw.UnmarshalYAML(node); err != nil {
		return err
	}
	return p.UnmarshalJSON(raw)
}

// UnmarshalYAML resolves aliases and merges before checking matcher fields.
func (m *SourceKeyMatch) UnmarshalYAML(node *yaml.Node) error {
	var raw common.RawJSON
	if err := raw.UnmarshalYAML(node); err != nil {
		return err
	}
	return m.UnmarshalJSON(raw)
}
