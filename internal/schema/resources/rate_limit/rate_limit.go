// Package rate_limit defines the canonical Kubernetes-style RateLimit resource
// and the policy types consumed by persistence and runtime enforcement.
package rate_limit

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/common"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	nschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
)

const (
	RateLimitKind  meta.Kind = "RateLimit"
	ConnectionKind meta.Kind = "Connection"
)

// Mode controls whether a matching rule rejects requests or only observes them.
type Mode string

const (
	ModeEnforce Mode = "enforce"
	ModeObserve Mode = "observe"
)

// DefaultMode is used when a RateLimit's Mode is unset.
const DefaultMode = ModeEnforce

// IsValidMode reports whether m is a recognised mode value.
func IsValidMode(m Mode) bool {
	switch m {
	case ModeEnforce, ModeObserve:
		return true
	default:
		return false
	}
}

// RateLimit is the Kubernetes-style representation of an AuthProxy rate-limit
// rule. Metadata.Namespace establishes the namespace cascade; Spec.Scope may
// narrow the rule to a namespace matcher, one connector, or one connection.
type RateLimit struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      meta.ObjectMeta  `json:"metadata" yaml:"metadata"`
	Spec          RateLimitSpec    `json:"spec" yaml:"spec"`
	Status        *RateLimitStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

// RateLimitSpec is desired rate-limit policy. This is the value serialized in
// the database definition column and consumed directly by enforcement.
type RateLimitSpec struct {
	Scope     *RateLimitScope `json:"scope,omitempty" yaml:"scope,omitempty"`
	Mode      Mode            `json:"mode,omitempty" yaml:"mode,omitempty"`
	Selector  Selector        `json:"selector" yaml:"selector"`
	Bucket    Bucket          `json:"bucket" yaml:"bucket"`
	Algorithm Algorithm       `json:"algorithm" yaml:"algorithm"`
}

// RateLimitScope narrows the metadata namespace cascade to one namespace
// matcher, connector, or connection. An omitted scope applies throughout the
// resource namespace and its descendants.
type RateLimitScope struct {
	NamespaceMatcher *string               `json:"namespaceMatcher,omitempty" yaml:"namespaceMatcher,omitempty"`
	ConnectorRef     *meta.ObjectReference `json:"connectorRef,omitempty" yaml:"connectorRef,omitempty"`
	ConnectionRef    *meta.ObjectReference `json:"connectionRef,omitempty" yaml:"connectionRef,omitempty"`
}

// RateLimitStatus contains server-observed policy state.
type RateLimitStatus struct {
	EffectiveMode Mode `json:"effectiveMode" yaml:"effectiveMode"`
}

// RateLimitPatch is a partial RateLimit used for updates. Metadata and Spec
// are required objects so omitted and null top-level fields are rejected.
type RateLimitPatch struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      *meta.ObjectMetaPatch `json:"metadata" yaml:"metadata"`
	Spec          *RateLimitSpecPatch   `json:"spec" yaml:"spec"`
	Status        *RateLimitStatus      `json:"status,omitempty" yaml:"status,omitempty"`
}

// RateLimitSpecPatch contains presence-aware desired-state updates. Scope may
// explicitly be null to restore namespace scope; other explicitly null fields
// are rejected instead of being mistaken for omission.
type RateLimitSpecPatch struct {
	Scope     *RateLimitScope `json:"scope,omitempty" yaml:"scope,omitempty"`
	Mode      *Mode           `json:"mode,omitempty" yaml:"mode,omitempty"`
	Selector  *Selector       `json:"selector,omitempty" yaml:"selector,omitempty"`
	Bucket    *Bucket         `json:"bucket,omitempty" yaml:"bucket,omitempty"`
	Algorithm *Algorithm      `json:"algorithm,omitempty" yaml:"algorithm,omitempty"`

	scopePresent     bool
	modePresent      bool
	selectorPresent  bool
	bucketPresent    bool
	algorithmPresent bool
}

func NewRateLimit() *RateLimit {
	return &RateLimit{TypeMeta: meta.NewTypeMeta(RateLimitKind)}
}

func NewRateLimitPatch() *RateLimitPatch {
	return &RateLimitPatch{
		TypeMeta: meta.NewTypeMeta(RateLimitKind),
		Metadata: &meta.ObjectMetaPatch{},
		Spec:     &RateLimitSpecPatch{},
	}
}

// GetId parses the resource's rate-limit ID, returning apid.Nil when absent or
// invalid.
func (r RateLimit) GetId() apid.ID {
	if r.Metadata.ID == "" {
		return apid.Nil
	}
	id, err := apid.Parse(r.Metadata.ID)
	if err != nil {
		return apid.Nil
	}
	return id
}

func (r *RateLimit) ApplyCreateDefaults(id apid.ID) *RateLimit {
	clone := r.Clone()
	if clone.Metadata.Name == "" {
		clone.Metadata.Name = common.ResourceName(id.String())
	}
	return clone
}

func (r *RateLimit) Clone() *RateLimit {
	if r == nil {
		return nil
	}
	clone := *r
	clone.Metadata = meta.CloneObjectMeta(r.Metadata)
	clone.Spec = r.Spec.Clone()
	if r.Status != nil {
		status := *r.Status
		clone.Status = &status
	}
	return &clone
}

func (s RateLimitSpec) Clone() RateLimitSpec {
	clone := s
	if s.Scope != nil {
		scope := *s.Scope
		if s.Scope.NamespaceMatcher != nil {
			matcher := *s.Scope.NamespaceMatcher
			scope.NamespaceMatcher = &matcher
		}
		if s.Scope.ConnectorRef != nil {
			ref := *s.Scope.ConnectorRef
			scope.ConnectorRef = &ref
		}
		if s.Scope.ConnectionRef != nil {
			ref := *s.Scope.ConnectionRef
			scope.ConnectionRef = &ref
		}
		clone.Scope = &scope
	}
	clone.Selector.Methods = append([]string(nil), s.Selector.Methods...)
	clone.Selector.RequestTypes = append([]common.RequestType(nil), s.Selector.RequestTypes...)
	clone.Bucket.Dimensions = append([]string(nil), s.Bucket.Dimensions...)
	return clone
}

// EffectiveMode returns Mode, falling back to DefaultMode when unset.
func (s *RateLimitSpec) EffectiveMode() Mode {
	if s == nil || s.Mode == "" {
		return DefaultMode
	}
	return s.Mode
}

// EffectiveNamespaceMatcher returns the explicit namespace scope, or the
// resource namespace and all descendants when scope is omitted or targets a
// connector or connection.
func (s *RateLimitSpec) EffectiveNamespaceMatcher(resourceNamespace string) string {
	if s != nil && s.Scope != nil && s.Scope.NamespaceMatcher != nil {
		return *s.Scope.NamespaceMatcher
	}
	return resourceNamespace + nschema.WildcardSuffix
}

// MatchesNamespace reports whether a request namespace is within this rule's
// effective namespace scope.
func (s *RateLimitSpec) MatchesNamespace(resourceNamespace, requestNamespace string) bool {
	return nschema.Matches(s.EffectiveNamespaceMatcher(resourceNamespace), requestNamespace)
}

func (s *RateLimitSpec) Equal(other *RateLimitSpec) bool {
	if s == nil && other == nil {
		return true
	}

	if s == nil || other == nil {
		return false
	}

	return reflect.DeepEqual(*s, *other)
}

func (r *RateLimit) Validate(vc *common.ValidationContext) error {
	return r.ValidateFor(meta.ValidationModeConfig, vc)
}

func (r *RateLimit) ValidateFor(mode meta.ValidationMode, vc *common.ValidationContext) error {
	if r == nil {
		return fmt.Errorf("rate limit is required")
	}
	if vc == nil {
		vc = &common.ValidationContext{Path: "$"}
	}
	requireStoredIdentity := mode == meta.ValidationModePersistence || mode == meta.ValidationModeResponse

	var result *multierror.Error
	if err := meta.ValidateResource(r.TypeMeta, r.Metadata, meta.ValidationOptions{
		Mode:               mode,
		Path:               vc,
		ExpectedAPIVersion: meta.APIVersionV1Alpha1,
		ExpectedKind:       RateLimitKind,
		RequireID:          requireStoredIdentity,
		RequireName:        requireStoredIdentity,
		RequireNamespace:   true,
		IDValidator:        ValidateID,
		NamespaceValidator: nschema.ValidatePath,
	}); err != nil {
		result = multierror.Append(result, err)
	}
	if r.Metadata.Generation != 0 {
		result = multierror.Append(result, vc.NewErrorForField("metadata.generation", "does not apply to rate limits"))
	}
	if err := r.Spec.validateForNamespace(r.Metadata.Namespace, vc.PushField("spec")); err != nil {
		result = multierror.Append(result, err)
	}
	if err := meta.ValidateStatus(r.Status, mode, vc); err != nil {
		result = multierror.Append(result, err)
	}
	if r.Status != nil && !IsValidMode(r.Status.EffectiveMode) {
		result = multierror.Append(result, vc.NewErrorForField("status.effectiveMode", "is not a recognized rate-limit mode"))
	}
	if (mode == meta.ValidationModePersistence || mode == meta.ValidationModeResponse) &&
		r.Status != nil && r.Status.EffectiveMode != r.Spec.EffectiveMode() {
		result = multierror.Append(result, vc.NewErrorForField("status.effectiveMode", "must match the effective spec mode"))
	}
	if requireStoredIdentity && r.Status == nil {
		result = multierror.Append(result, vc.NewErrorForField("status", "is required"))
	}
	return result.ErrorOrNil()
}

// Validate retains the policy-only validation entry point used by persistence
// and enforcement code.
func (s *RateLimitSpec) Validate() error {
	if s == nil {
		return fmt.Errorf("rate-limit spec is required")
	}
	return s.validate(&common.ValidationContext{})
}

// ValidateForNamespace validates the policy and confirms that its scope cannot
// escape the namespace that owns the RateLimit resource.
func (s *RateLimitSpec) ValidateForNamespace(resourceNamespace string) error {
	if s == nil {
		return fmt.Errorf("rate-limit spec is required")
	}
	return s.validateForNamespace(resourceNamespace, &common.ValidationContext{})
}

func (s *RateLimitSpec) validateForNamespace(resourceNamespace string, vc *common.ValidationContext) error {
	var result *multierror.Error
	if err := s.validate(vc); err != nil {
		result = multierror.Append(result, err)
	}
	if err := ValidateScopeNamespaceBoundary(s.Scope, resourceNamespace, vc); err != nil {
		result = multierror.Append(result, err)
	}
	return result.ErrorOrNil()
}

func (s *RateLimitSpec) validate(vc *common.ValidationContext) error {
	var result *multierror.Error
	if s.Mode != "" && !IsValidMode(s.Mode) {
		result = multierror.Append(result, vc.NewErrorfForField("mode", "invalid mode %q", string(s.Mode)))
	}
	if err := ValidateScope(s.Scope, vc); err != nil {
		result = multierror.Append(result, err)
	}
	if err := s.Selector.Validate(vc.PushField("selector")); err != nil {
		result = multierror.Append(result, err)
	}
	if err := s.Bucket.Validate(vc.PushField("bucket")); err != nil {
		result = multierror.Append(result, err)
	}
	if err := s.Algorithm.Validate(vc.PushField("algorithm")); err != nil {
		result = multierror.Append(result, err)
	}
	return result.ErrorOrNil()
}

func ValidateScope(scope *RateLimitScope, vc *common.ValidationContext) error {
	if scope == nil {
		return nil
	}
	if vc == nil {
		vc = &common.ValidationContext{}
	}
	var result *multierror.Error
	count := 0
	if scope.NamespaceMatcher != nil {
		count++
		if err := nschema.ValidateMatcher(*scope.NamespaceMatcher); err != nil {
			result = multierror.Append(result, vc.NewErrorfForField("scope.namespaceMatcher", "%v", err))
		}
	}
	if scope.ConnectorRef != nil {
		count++
		if err := meta.ValidateObjectReferenceWithOptions(*scope.ConnectorRef, meta.ObjectReferenceValidationOptions{
			ExpectedAPIVersion: meta.APIVersionV1Alpha1,
			ExpectedKind:       cschema.ConnectorKind,
			IDValidator:        validateConnectorID,
			NamespaceValidator: nschema.ValidatePath,
		}, vc.PushField("scope").PushField("connectorRef")); err != nil {
			result = multierror.Append(result, err)
		}
		if scope.ConnectorRef.Generation != 0 {
			result = multierror.Append(result, vc.NewErrorForField("scope.connectorRef.generation", "does not apply to rate-limit connector references"))
		}
	}
	if scope.ConnectionRef != nil {
		count++
		if err := meta.ValidateObjectReferenceWithOptions(*scope.ConnectionRef, meta.ObjectReferenceValidationOptions{
			ExpectedAPIVersion: meta.APIVersionV1Alpha1,
			ExpectedKind:       ConnectionKind,
			IDValidator:        validateConnectionID,
			NamespaceValidator: nschema.ValidatePath,
		}, vc.PushField("scope").PushField("connectionRef")); err != nil {
			result = multierror.Append(result, err)
		}
		if scope.ConnectionRef.Generation != 0 {
			result = multierror.Append(result, vc.NewErrorForField("scope.connectionRef.generation", "does not apply to connections"))
		}
	}
	if count != 1 {
		result = multierror.Append(result, vc.NewErrorForField("scope", "must contain exactly one of namespaceMatcher, connectorRef, or connectionRef"))
	}
	return result.ErrorOrNil()
}

// ValidateScopeNamespaceBoundary ensures an explicit namespace matcher or a
// namespaced reference stays at or below the namespace that owns the rule.
// ID-only references are checked against their resolved target in core.
func ValidateScopeNamespaceBoundary(scope *RateLimitScope, resourceNamespace string, vc *common.ValidationContext) error {
	if scope == nil {
		return nil
	}
	if vc == nil {
		vc = &common.ValidationContext{}
	}
	var result *multierror.Error
	if scope.NamespaceMatcher != nil {
		base := strings.TrimSuffix(*scope.NamespaceMatcher, nschema.WildcardSuffix)
		if !nschema.IsSameOrChild(resourceNamespace, base) {
			result = multierror.Append(result, vc.NewErrorfForField(
				"scope.namespaceMatcher",
				"must match only namespace %q or its descendants",
				resourceNamespace,
			))
		}
	}
	if ref := scope.ConnectorRef; ref != nil && ref.Namespace != "" && !nschema.IsSameOrChild(resourceNamespace, ref.Namespace) {
		result = multierror.Append(result, vc.NewErrorfForField(
			"scope.connectorRef.namespace",
			"must be namespace %q or one of its descendants",
			resourceNamespace,
		))
	}
	if ref := scope.ConnectionRef; ref != nil && ref.Namespace != "" && !nschema.IsSameOrChild(resourceNamespace, ref.Namespace) {
		result = multierror.Append(result, vc.NewErrorfForField(
			"scope.connectionRef.namespace",
			"must be namespace %q or one of its descendants",
			resourceNamespace,
		))
	}
	return result.ErrorOrNil()
}

func ValidateID(value string) error {
	return validatePrefixedID(value, apid.PrefixRateLimit, "rate limit")
}
func validateConnectorID(value string) error {
	return validatePrefixedID(value, apid.PrefixConnector, "connector")
}
func validateConnectionID(value string) error {
	return validatePrefixedID(value, apid.PrefixConnection, "connection")
}

func validatePrefixedID(value string, prefix apid.Prefix, name string) error {
	id, err := apid.Parse(value)
	if err != nil {
		return err
	}
	if id.Prefix() != prefix {
		return fmt.Errorf("must be a %s id", name)
	}
	return nil
}

func (p *RateLimitPatch) ValidateFor(mode meta.ValidationMode, vc *common.ValidationContext) error {
	if p == nil {
		return fmt.Errorf("rate-limit patch is required")
	}
	if vc == nil {
		vc = &common.ValidationContext{Path: "$"}
	}
	var result *multierror.Error
	if err := meta.ValidateTypeMeta(p.TypeMeta, meta.APIVersionV1Alpha1, RateLimitKind, vc); err != nil {
		result = multierror.Append(result, err)
	}
	if p.Metadata == nil {
		result = multierror.Append(result, vc.NewErrorForField("metadata", "is required and must not be null"))
	} else if err := meta.ValidateObjectMetaPatch(*p.Metadata, meta.ValidationOptions{
		Mode: mode, Path: vc, IDValidator: ValidateID, NamespaceValidator: nschema.ValidatePath,
	}); err != nil {
		result = multierror.Append(result, err)
	}
	if p.Spec == nil {
		result = multierror.Append(result, vc.NewErrorForField("spec", "is required and must not be null"))
	} else if err := p.Spec.validate(vc.PushField("spec")); err != nil {
		result = multierror.Append(result, err)
	}
	if err := meta.ValidateStatus(p.Status, mode, vc); err != nil {
		result = multierror.Append(result, err)
	}
	return result.ErrorOrNil()
}

func (p *RateLimitSpecPatch) validate(vc *common.ValidationContext) error {
	var result *multierror.Error
	if p.modePresent && p.Mode == nil {
		result = multierror.Append(result, vc.NewErrorForField("mode", "must not be null"))
	} else if p.Mode != nil && !IsValidMode(*p.Mode) {
		result = multierror.Append(result, vc.NewErrorForField("mode", "is not a recognized rate-limit mode"))
	}
	if p.scopePresent && p.Scope != nil {
		result = multierror.Append(result, ValidateScope(p.Scope, vc))
	}
	if p.selectorPresent && p.Selector == nil {
		result = multierror.Append(result, vc.NewErrorForField("selector", "must not be null"))
	} else if p.Selector != nil {
		result = multierror.Append(result, p.Selector.Validate(vc.PushField("selector")))
	}
	if p.bucketPresent && p.Bucket == nil {
		result = multierror.Append(result, vc.NewErrorForField("bucket", "must not be null"))
	} else if p.Bucket != nil {
		result = multierror.Append(result, p.Bucket.Validate(vc.PushField("bucket")))
	}
	if p.algorithmPresent && p.Algorithm == nil {
		result = multierror.Append(result, vc.NewErrorForField("algorithm", "must not be null"))
	} else if p.Algorithm != nil {
		result = multierror.Append(result, p.Algorithm.Validate(vc.PushField("algorithm")))
	}
	return result.ErrorOrNil()
}

// SetScope records an explicit scope update. A nil scope restores namespace
// scope and is serialized as null.
func (p *RateLimitSpecPatch) SetScope(scope *RateLimitScope) {
	if p == nil {
		return
	}
	p.Scope = scope
	p.scopePresent = true
}

// HasScope reports whether scope was supplied, including an explicit null.
func (p *RateLimitSpecPatch) HasScope() bool {
	return p != nil && (p.scopePresent || p.Scope != nil)
}

func (p *RateLimitPatch) ApplyTo(current *RateLimit, vc *common.ValidationContext) (*RateLimit, error) {
	if current == nil {
		return nil, fmt.Errorf("current rate limit is required")
	}
	if vc == nil {
		vc = &common.ValidationContext{Path: "$"}
	}
	if err := p.ValidateFor(meta.ValidationModeUpdate, vc); err != nil {
		return nil, err
	}
	updated := current.Clone()
	updated.Metadata = meta.ApplyObjectMetaPatch(updated.Metadata, *p.Metadata)
	if p.Spec.HasScope() {
		updated.Spec.Scope = nil
		if p.Spec.Scope != nil {
			scope := *p.Spec.Scope
			updated.Spec.Scope = &scope
		}
	}
	if p.Spec.Mode != nil {
		updated.Spec.Mode = *p.Spec.Mode
	}
	if p.Spec.Selector != nil {
		updated.Spec.Selector = *p.Spec.Selector
	}
	if p.Spec.Bucket != nil {
		updated.Spec.Bucket = *p.Spec.Bucket
	}
	if p.Spec.Algorithm != nil {
		updated.Spec.Algorithm = *p.Spec.Algorithm
	}
	if err := updated.Spec.validateForNamespace(updated.Metadata.Namespace, vc.PushField("spec")); err != nil {
		return nil, err
	}
	if err := ValidateUpdate(current, updated, vc); err != nil {
		return nil, err
	}
	return updated, nil
}

func ValidateUpdate(before, after *RateLimit, vc *common.ValidationContext) error {
	if before == nil || after == nil {
		return fmt.Errorf("before and after rate limits are required")
	}
	if vc == nil {
		vc = &common.ValidationContext{Path: "$"}
	}
	var result *multierror.Error
	if err := meta.ValidateTypeMetaUpdate(before.TypeMeta, after.TypeMeta, vc); err != nil {
		result = multierror.Append(result, err)
	}
	if err := meta.ValidateMetadataUpdate(before.Metadata, after.Metadata, meta.UpdateOptions{ImmutableNamespace: true}, vc); err != nil {
		result = multierror.Append(result, err)
	}
	return result.ErrorOrNil()
}
