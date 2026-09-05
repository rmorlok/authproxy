package actor

import (
	"fmt"
	"log/slog"
	"slices"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apid"
	authschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/schema/common"
	keyschema "github.com/rmorlok/authproxy/internal/schema/resources/key"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	nschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
)

// Actor is the canonical Kubernetes-style representation of an AuthProxy
// identity. SigningKey is accepted as write-only configuration and is never
// populated in a response resource.
type Actor struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      meta.ObjectMeta `json:"metadata" yaml:"metadata"`
	Spec          ActorSpec       `json:"spec" yaml:"spec"`
	Status        *ActorStatus    `json:"status,omitempty" yaml:"status,omitempty"`
}

// ActorSpec contains the external subject, base permissions, and optional
// actor-specific signing material.
type ActorSpec struct {
	ExternalId  string                  `json:"externalId" yaml:"externalId"`
	Permissions []authschema.Permission `json:"permissions,omitempty" yaml:"permissions,omitempty"`
	SigningKey  *keyschema.SigningKey   `json:"signingKey,omitempty" yaml:"signingKey,omitempty"`
}

// ActorStatus contains safe, server-observed actor state.
type ActorStatus struct {
	SigningKeyConfigured bool `json:"signingKeyConfigured" yaml:"signingKeyConfigured"`
}

// ActorPatch is a presence-aware Actor update. Metadata and Spec must be
// present even when their nested fields are omitted.
type ActorPatch struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      *meta.ObjectMetaPatch `json:"metadata" yaml:"metadata"`
	Spec          *ActorSpecPatch       `json:"spec" yaml:"spec"`
	Status        *ActorStatus          `json:"status,omitempty" yaml:"status,omitempty"`
}

// ActorSpecPatch contains mutable Actor spec updates. Presence flags retain
// the distinction between omission and explicit null values.
type ActorSpecPatch struct {
	ExternalId  *string                  `json:"externalId,omitempty" yaml:"externalId,omitempty"`
	Permissions *[]authschema.Permission `json:"permissions,omitempty" yaml:"permissions,omitempty"`
	SigningKey  *keyschema.SigningKey    `json:"signingKey,omitempty" yaml:"signingKey,omitempty"`

	externalIdPresent  bool
	permissionsPresent bool
	signingKeyPresent  bool
}

// NewActor returns an empty Actor with registered type metadata.
func NewActor() *Actor {
	return &Actor{TypeMeta: meta.NewTypeMeta(ActorKind)}
}

// NewActorPatch returns an empty Actor update envelope.
func NewActorPatch() *ActorPatch {
	return &ActorPatch{
		TypeMeta: meta.NewTypeMeta(ActorKind),
		Metadata: &meta.ObjectMetaPatch{},
		Spec:     &ActorSpecPatch{},
	}
}

// ApplyCreateDefaults returns a copy with a server-allocated default name.
func (a *Actor) ApplyCreateDefaults(id apid.ID) *Actor {
	clone := a.Clone()
	if clone.Metadata.Name == "" {
		clone.Metadata.Name = common.ResourceName(id.String())
	}
	return clone
}

// Clone returns a deep copy of the Actor's mutable metadata and permissions.
// Signing-key implementations are immutable configuration values and may be
// shared by the copy.
func (a *Actor) Clone() *Actor {
	if a == nil {
		return nil
	}
	clone := *a
	clone.Metadata = meta.CloneObjectMeta(a.Metadata)
	clone.Spec.Permissions = ClonePermissions(a.Spec.Permissions)
	if a.Status != nil {
		status := *a.Status
		clone.Status = &status
	}
	return &clone
}

// ClonePermissions returns a deep copy of the Actor permission list,
// including each permission's resources, resource IDs, and verbs.
func ClonePermissions(permissions []authschema.Permission) []authschema.Permission {
	result := slices.Clone(permissions)
	for i := range result {
		result[i].Resources = slices.Clone(result[i].Resources)
		result[i].ResourceIds = slices.Clone(result[i].ResourceIds)
		result[i].Verbs = slices.Clone(result[i].Verbs)
	}
	return result
}

func (a *Actor) GetId() apid.ID {
	if a == nil || a.Metadata.ID == "" {
		return apid.Nil
	}
	id, err := apid.Parse(a.Metadata.ID)
	if err != nil {
		return apid.Nil
	}
	return id
}

func (a *Actor) GetName() common.ResourceName {
	if a == nil {
		return ""
	}
	if a.Metadata.Name == "" && a.Metadata.ID != "" {
		return common.ResourceName(a.Metadata.ID)
	}
	return a.Metadata.Name
}

func (a *Actor) GetExternalId() string {
	if a == nil {
		return ""
	}
	return a.Spec.ExternalId
}

func (a *Actor) GetPermissions() []authschema.Permission {
	if a == nil {
		return nil
	}
	return a.Spec.Permissions
}

func (a *Actor) GetNamespace() string {
	if a == nil {
		return ""
	}
	return a.Metadata.Namespace
}

func (a *Actor) GetLabels() map[string]string {
	if a == nil {
		return nil
	}
	return a.Metadata.Labels
}

func (a *Actor) GetAnnotations() map[string]string {
	if a == nil {
		return nil
	}
	return a.Metadata.Annotations
}

func (a *ActorSpecPatch) HasExternalId() bool {
	return a != nil && (a.externalIdPresent || a.ExternalId != nil)
}

func (a *ActorSpecPatch) HasPermissions() bool {
	return a != nil && (a.permissionsPresent || a.Permissions != nil)
}

func (a *ActorSpecPatch) HasSigningKey() bool {
	return a != nil && (a.signingKeyPresent || a.SigningKey != nil)
}

func (a *ActorSpecPatch) SetSigningKey(value *keyschema.SigningKey) {
	if a == nil {
		return
	}
	a.SigningKey = value
	a.signingKeyPresent = true
}

// Validate applies configuration-file Actor rules.
func (a *Actor) Validate(vc *common.ValidationContext) error {
	return a.ValidateFor(meta.ValidationModeConfig, vc)
}

// ValidateFor validates the Actor at one resource lifecycle boundary.
func (a *Actor) ValidateFor(mode meta.ValidationMode, vc *common.ValidationContext) error {
	if a == nil {
		return fmt.Errorf("actor is required")
	}
	if vc == nil {
		vc = &common.ValidationContext{Path: "$"}
	}

	requireStoredIdentity := mode == meta.ValidationModePersistence || mode == meta.ValidationModeResponse
	var result *multierror.Error
	if err := meta.ValidateResource(a.TypeMeta, a.Metadata, meta.ValidationOptions{
		Mode:               mode,
		Path:               vc,
		ExpectedAPIVersion: meta.APIVersionV1Alpha1,
		ExpectedKind:       ActorKind,
		RequireID:          requireStoredIdentity,
		RequireName:        requireStoredIdentity,
		RequireNamespace:   true,
		IDValidator:        ValidateID,
		NamespaceValidator: nschema.ValidatePath,
	}); err != nil {
		result = multierror.Append(result, err)
	}

	if a.Metadata.Generation != 0 {
		result = multierror.Append(result, vc.NewErrorForField("metadata.generation", "does not apply to actors"))
	}
	if a.Spec.ExternalId == "" {
		result = multierror.Append(result, vc.NewErrorForField("spec.externalId", "is required"))
	}
	result = appendPermissionErrors(result, a.Spec.Permissions, vc.PushField("spec").PushField("permissions"))

	if a.Spec.SigningKey != nil && a.Spec.SigningKey.InnerVal == nil {
		result = multierror.Append(result, vc.NewErrorForField("spec.signingKey", "must contain signing-key configuration"))
	}
	if mode == meta.ValidationModeConfig && a.Spec.SigningKey == nil {
		result = multierror.Append(result, vc.NewErrorForField("spec.signingKey", "is required for configured actors"))
	}
	if requireStoredIdentity && a.Spec.SigningKey != nil {
		result = multierror.Append(result, vc.NewErrorForField("spec.signingKey", "must be redacted from stored and response resources"))
	}

	if err := meta.ValidateStatus(a.Status, mode, vc); err != nil {
		result = multierror.Append(result, err)
	}
	if requireStoredIdentity && a.Status == nil {
		result = multierror.Append(result, vc.NewErrorForField("status", "is required"))
	}

	return result.ErrorOrNil()
}

// ValidateFor validates a partial Actor at the API update boundary.
func (a *ActorPatch) ValidateFor(mode meta.ValidationMode, vc *common.ValidationContext) error {
	if a == nil {
		return fmt.Errorf("actor patch is required")
	}
	if vc == nil {
		vc = &common.ValidationContext{Path: "$"}
	}

	var result *multierror.Error
	if err := meta.ValidateTypeMeta(a.TypeMeta, meta.APIVersionV1Alpha1, ActorKind, vc); err != nil {
		result = multierror.Append(result, err)
	}
	if a.Metadata == nil {
		result = multierror.Append(result, vc.NewErrorForField("metadata", "is required and must not be null"))
	} else {
		if err := meta.ValidateObjectMetaPatch(*a.Metadata, meta.ValidationOptions{
			Mode:               mode,
			Path:               vc,
			IDValidator:        ValidateID,
			NamespaceValidator: nschema.ValidatePath,
		}); err != nil {
			result = multierror.Append(result, err)
		}
		if a.Metadata.Generation != nil {
			result = multierror.Append(result, vc.NewErrorForField("metadata.generation", "does not apply to actors"))
		}
	}
	if a.Spec == nil {
		result = multierror.Append(result, vc.NewErrorForField("spec", "is required and must not be null"))
	} else {
		if a.Spec.HasExternalId() {
			if a.Spec.ExternalId == nil {
				result = multierror.Append(result, vc.NewErrorForField("spec.externalId", "must not be null"))
			} else if *a.Spec.ExternalId == "" {
				result = multierror.Append(result, vc.NewErrorForField("spec.externalId", "must not be empty"))
			}
		}
		if a.Spec.HasPermissions() {
			if a.Spec.Permissions == nil {
				result = multierror.Append(result, vc.NewErrorForField("spec.permissions", "must not be null"))
			} else {
				result = appendPermissionErrors(result, *a.Spec.Permissions, vc.PushField("spec").PushField("permissions"))
			}
		}
		if a.Spec.HasSigningKey() && a.Spec.SigningKey != nil && a.Spec.SigningKey.InnerVal == nil {
			result = multierror.Append(result, vc.NewErrorForField("spec.signingKey", "must contain signing-key configuration or be null"))
		}
	}
	if err := meta.ValidateStatus(a.Status, mode, vc); err != nil {
		result = multierror.Append(result, err)
	}

	return result.ErrorOrNil()
}

// ApplyTo applies the patch to a copy and enforces Actor immutability rules.
func (a *ActorPatch) ApplyTo(current *Actor, vc *common.ValidationContext) (*Actor, error) {
	if current == nil {
		return nil, fmt.Errorf("current actor is required")
	}
	if err := a.ValidateFor(meta.ValidationModeUpdate, vc); err != nil {
		return nil, err
	}

	updated := current.Clone()
	updated.Metadata = meta.ApplyObjectMetaPatch(updated.Metadata, *a.Metadata)
	if a.Spec.HasExternalId() {
		updated.Spec.ExternalId = *a.Spec.ExternalId
	}
	if a.Spec.HasPermissions() {
		updated.Spec.Permissions = ClonePermissions(*a.Spec.Permissions)
	}
	if a.Spec.HasSigningKey() {
		updated.Spec.SigningKey = a.Spec.SigningKey
	}

	if err := ValidateUpdate(current, updated, vc); err != nil {
		return nil, err
	}
	return updated, nil
}

// ValidateUpdate rejects changes to immutable Actor identity.
func ValidateUpdate(before, after *Actor, vc *common.ValidationContext) error {
	if before == nil || after == nil {
		return fmt.Errorf("before and after actors are required")
	}
	if vc == nil {
		vc = &common.ValidationContext{Path: "$"}
	}

	var result *multierror.Error
	if err := meta.ValidateTypeMetaUpdate(before.TypeMeta, after.TypeMeta, vc); err != nil {
		result = multierror.Append(result, err)
	}
	if err := meta.ValidateMetadataUpdate(before.Metadata, after.Metadata, meta.UpdateOptions{
		ImmutableNamespace: true,
	}, vc); err != nil {
		result = multierror.Append(result, err)
	}
	if before.Spec.ExternalId != after.Spec.ExternalId {
		result = multierror.Append(result, vc.NewErrorForField("spec.externalId", "is immutable"))
	}
	return result.ErrorOrNil()
}

// ValidateID verifies an immutable Actor identifier.
func ValidateID(value string) error {
	id, err := apid.Parse(value)
	if err != nil {
		return err
	}
	if id.Prefix() != apid.PrefixActor {
		return fmt.Errorf("must be an actor id")
	}
	return nil
}

func appendPermissionErrors(
	result *multierror.Error,
	permissions []authschema.Permission,
	vc *common.ValidationContext,
) *multierror.Error {
	for i := range permissions {
		if err := permissions[i].Validate(); err != nil {
			result = multierror.Append(result, vc.PushIndex(i).NewError(err.Error()))
		}
	}
	return result
}

// LogValue emits safe Actor identity and policy fields without signing-key
// material.
func (a Actor) LogValue() slog.Value {
	signingKeyConfigured := a.Spec.SigningKey != nil
	if a.Status != nil {
		signingKeyConfigured = a.Status.SigningKeyConfigured
	}
	return slog.GroupValue(
		slog.String("apiVersion", string(a.APIVersion)),
		slog.String("kind", string(a.Kind)),
		slog.String("id", a.Metadata.ID),
		slog.String("name", string(a.Metadata.Name)),
		slog.String("namespace", a.Metadata.Namespace),
		slog.String("externalId", a.Spec.ExternalId),
		slog.Int("permissionCount", len(a.Spec.Permissions)),
		slog.Bool("signingKeyConfigured", signingKeyConfigured),
	)
}
