package toolsets

import (
	"encoding/json"
	"fmt"
	"maps"
	"sort"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	nschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/util"
	"gopkg.in/yaml.v3"
)

// ConnectionSelector selects connections within the owning ToolSet's namespace
// subtree. Omitted or empty Namespace means the owner and all descendants.
// MatchLabels is required: an explicit empty map selects every connection in
// scope, while a missing/null map is invalid. Every supplied label must match
// exactly, including key presence for empty values. Inherited and system labels
// may be selected; this contract never writes labels or grants authorization.
type ConnectionSelector struct {
	Namespace   string            `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	MatchLabels map[string]string `json:"matchLabels" yaml:"matchLabels"`
}

// ValidateForNamespace checks namespace scope and exact label requirements.
// Explicit matchers may narrow the owner's subtree but cannot select ancestors
// or siblings. Labels use read-side validation so system and inherited keys
// retain their existing grammar and longer system-value limits.
func (s *ConnectionSelector) ValidateForNamespace(owner string, vc *common.ValidationContext) error {
	vc = validationContext(vc)
	if s == nil {
		return vc.NewError("connection selector is required")
	}
	var result *multierror.Error
	ownerErr := nschema.ValidatePath(owner)
	if ownerErr != nil {
		result = multierror.Append(result, vc.NewErrorf("invalid owning namespace: %v", ownerErr))
	}
	if s.Namespace != "" {
		if err := nschema.ValidateMatcher(s.Namespace); err != nil {
			result = multierror.Append(result, vc.NewErrorfForField("namespace", "%v", err))
		} else if ownerErr == nil && !nschema.IsSameOrChild(owner, strings.TrimSuffix(s.Namespace, nschema.WildcardSuffix)) {
			result = multierror.Append(result, vc.NewErrorfForField("namespace", "must match only namespace %q or its descendants", owner))
		}
	}
	if s.MatchLabels == nil {
		result = multierror.Append(result, vc.NewErrorForField("matchLabels", "is required and must be an object; use {} to select all connections in scope"))
	} else if err := meta.ValidateLabels(s.MatchLabels); err != nil {
		result = multierror.Append(result, vc.NewErrorfForField("matchLabels", "%v", err))
	}
	return result.ErrorOrNil()
}

// Compile returns inputs for the existing namespace and label matchers without
// depending on their database implementation. Validated keys and values cannot
// contain selector delimiters, so sorted key=value terms express exactly ANDed
// equality requirements. An empty label string imposes no label restriction;
// the returned namespace matcher still applies. Errors return no partial scope.
func (s *ConnectionSelector) Compile(owner string) (namespaceMatcher, labelSelector string, err error) {
	if err := s.ValidateForNamespace(owner, nil); err != nil {
		return "", "", err
	}
	namespaceMatcher = s.Namespace
	if namespaceMatcher == "" {
		namespaceMatcher = owner + nschema.WildcardSuffix
	}
	keys := make([]string, 0, len(s.MatchLabels))
	for key := range s.MatchLabels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+s.MatchLabels[key])
	}
	return namespaceMatcher, strings.Join(parts, ","), nil
}

// Clone returns an independent selector, retaining a nil label map so cloning
// cannot turn a missing selection policy into an explicit empty map.
func (s *ConnectionSelector) Clone() *ConnectionSelector {
	if s == nil {
		return nil
	}
	clone := *s
	clone.MatchLabels = maps.Clone(s.MatchLabels)
	return &clone
}

// UnmarshalJSON rejects unknown fields and null string values before decoding
// can mistake them for empty equality requirements. Missing/null matchLabels
// remains nil for validation; an explicit {} remains a non-nil empty map.
func (s *ConnectionSelector) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := util.DecodeJSONStrict(data, &fields); err != nil {
		return err
	}
	if fields == nil {
		return fmt.Errorf("connection selector must be an object")
	}
	var decoded ConnectionSelector
	for name, raw := range fields {
		switch name {
		case "namespace":
			var namespace *string
			if err := util.DecodeJSONStrict(raw, &namespace); err != nil {
				return fmt.Errorf("decode namespace: %w", err)
			}
			if namespace == nil {
				return fmt.Errorf("namespace must be a string, not null")
			}
			decoded.Namespace = *namespace
		case "matchLabels":
			var labels map[string]*string
			if err := util.DecodeJSONStrict(raw, &labels); err != nil {
				return fmt.Errorf("decode matchLabels: %w", err)
			}
			if labels != nil {
				decoded.MatchLabels = make(map[string]string, len(labels))
				for key, value := range labels {
					if value == nil {
						return fmt.Errorf("matchLabels[%q] must be a string, not null", key)
					}
					decoded.MatchLabels[key] = *value
				}
			}
		default:
			return fmt.Errorf("unknown connection selector field %q", name)
		}
	}
	*s = decoded
	return nil
}

// UnmarshalYAML resolves aliases and merge keys before applying the same strict
// JSON rules. Null labels cannot silently become empty string requirements.
func (s *ConnectionSelector) UnmarshalYAML(node *yaml.Node) error {
	var raw common.RawJSON
	if err := raw.UnmarshalYAML(node); err != nil {
		return err
	}
	return s.UnmarshalJSON(raw)
}
