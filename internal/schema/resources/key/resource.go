package key

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apserde"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	nschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
)

const KeyKind meta.Kind = "Key"

// KeyUsage describes what a managed key protects.
type KeyUsage string

const (
	KeyUsageDataEncryption KeyUsage = "data_encryption"
)

// KeyMaterialType describes where and how a managed key's wrapping material
// is held.
type KeyMaterialType string

const (
	KeyMaterialTypeSymmetric KeyMaterialType = "symmetric"
	KeyMaterialTypePublic    KeyMaterialType = "public"
	KeyMaterialTypePrivate   KeyMaterialType = "private"
	KeyMaterialTypeExternal  KeyMaterialType = "external"
)

// KeyState is the desired or observed lifecycle state of a managed key.
type KeyState string

const (
	KeyStateActive   KeyState = "active"
	KeyStateDisabled KeyState = "disabled"
)

// Key is the Kubernetes-style representation of an AuthProxy-managed key.
// Provider configuration is part of desired state, but fields tagged as
// secrets must be redacted before a Key is returned by an API boundary.
type Key struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      meta.ObjectMeta `json:"metadata" yaml:"metadata"`
	Spec          KeySpec         `json:"spec" yaml:"spec"`
	Status        *KeyStatus      `json:"status,omitempty" yaml:"status,omitempty"`
}

// KeySpec contains desired key policy and provider configuration.
type KeySpec struct {
	Usage        KeyUsage        `json:"usage,omitempty" yaml:"usage,omitempty"`
	MaterialType KeyMaterialType `json:"materialType,omitempty" yaml:"materialType,omitempty"`
	DesiredState KeyState        `json:"desiredState,omitempty" yaml:"desiredState,omitempty"`
	KeyData      *KeyData        `json:"keyData,omitempty" yaml:"keyData,omitempty"`
}

// KeyStatus contains server-observed key state. KeyDataConfigured reports
// whether encrypted provider configuration exists without exposing it.
type KeyStatus struct {
	State             KeyState `json:"state" yaml:"state"`
	KeyDataConfigured bool     `json:"keyDataConfigured" yaml:"keyDataConfigured"`
}

// KeyPatch is a partial managed Key used for updates. Metadata and Spec are
// required objects so omitted and null top-level fields are rejected.
type KeyPatch struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      *meta.ObjectMetaPatch `json:"metadata" yaml:"metadata"`
	Spec          *KeySpecPatch         `json:"spec" yaml:"spec"`
	Status        *KeyStatus            `json:"status,omitempty" yaml:"status,omitempty"`
}

// KeySpecPatch contains presence-aware desired-state updates.
type KeySpecPatch struct {
	Usage        *KeyUsage        `json:"usage,omitempty" yaml:"usage,omitempty"`
	MaterialType *KeyMaterialType `json:"materialType,omitempty" yaml:"materialType,omitempty"`
	DesiredState *KeyState        `json:"desiredState,omitempty" yaml:"desiredState,omitempty"`
	KeyData      *KeyData         `json:"keyData,omitempty" yaml:"keyData,omitempty"`

	keyDataPresent bool
}

// NewKey returns an empty managed Key with registered type metadata.
func NewKey() *Key {
	return &Key{TypeMeta: meta.NewTypeMeta(KeyKind)}
}

// NewKeyPatch returns an empty managed Key update envelope.
func NewKeyPatch() *KeyPatch {
	return &KeyPatch{
		TypeMeta: meta.NewTypeMeta(KeyKind),
		Metadata: &meta.ObjectMetaPatch{},
		Spec:     &KeySpecPatch{},
	}
}

// GetId parses the resource's managed key ID, returning apid.Nil when absent
// or invalid.
func (k Key) GetId() apid.ID {
	if k.Metadata.ID == "" {
		return apid.Nil
	}
	id, err := apid.Parse(k.Metadata.ID)
	if err != nil {
		return apid.Nil
	}
	return id
}

// ApplyCreateDefaults returns a copy with defaults for policy fields. Name is
// defaulted to the allocated ID when the caller did not provide one.
func (k *Key) ApplyCreateDefaults(id apid.ID) (*Key, error) {
	clone, err := k.Clone()
	if err != nil {
		return nil, err
	}
	if clone.Metadata.Name == "" {
		clone.Metadata.Name = common.ResourceName(id.String())
	}
	if clone.Spec.Usage == "" {
		clone.Spec.Usage = KeyUsageDataEncryption
	}
	if clone.Spec.MaterialType == "" {
		clone.Spec.MaterialType = KeyMaterialTypeSymmetric
	}
	if clone.Spec.DesiredState == "" {
		clone.Spec.DesiredState = KeyStateActive
	}
	return clone, nil
}

// Clone returns a deep copy of k, including provider configuration.
func (k *Key) Clone() (*Key, error) {
	if k == nil {
		return nil, nil
	}

	clone := *k
	clone.Metadata = meta.CloneObjectMeta(k.Metadata)
	if k.Spec.KeyData != nil {
		// Provider implementations may contain runtime clients, mutexes, or raw
		// in-memory material that intentionally is not serializable. Treat the
		// immutable provider implementation as shared while copying its wrapper.
		keyData := *k.Spec.KeyData
		clone.Spec.KeyData = &keyData
	}
	if k.Status != nil {
		status := *k.Status
		clone.Status = &status
	}
	return &clone, nil
}

// RedactKeyData returns a structurally equivalent provider configuration with
// all secret-tagged values masked. It intentionally uses a context without
// secret-replay authorization: managed key material is always write-only,
// including for callers that may replay other resource secrets.
func RedactKeyData(value *KeyData) (*KeyData, error) {
	if value == nil {
		return nil, nil
	}
	if value.GetProviderType() == ProviderTypeRaw {
		// Raw key data exists only as runtime in-memory bytes and has no safe
		// serialized provider configuration to return.
		return nil, nil
	}

	sanitized, _, err := apserde.SanitizeJSONForAPI(context.Background(), value)
	if err != nil {
		return nil, fmt.Errorf("redact key data: %w", err)
	}
	data, err := json.Marshal(sanitized)
	if err != nil {
		return nil, fmt.Errorf("marshal redacted key data: %w", err)
	}
	var result KeyData
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decode redacted key data: %w", err)
	}
	return &result, nil
}

// HasKeyData reports whether keyData was explicitly supplied in a patch.
func (p *KeySpecPatch) HasKeyData() bool {
	return p != nil && (p.keyDataPresent || p.KeyData != nil)
}

// SetKeyData records an explicit provider-configuration update.
func (p *KeySpecPatch) SetKeyData(value *KeyData) {
	if p == nil {
		return
	}
	p.KeyData = value
	p.keyDataPresent = true
}

// Validate applies configuration-file resource rules. API handlers use
// ValidateFor with the lifecycle mode appropriate to the request.
func (k *Key) Validate(vc *common.ValidationContext) error {
	return k.ValidateFor(meta.ValidationModeConfig, vc)
}

// ValidateFor validates k for one resource lifecycle boundary.
func (k *Key) ValidateFor(mode meta.ValidationMode, vc *common.ValidationContext) error {
	if k == nil {
		return fmt.Errorf("key is required")
	}
	if vc == nil {
		vc = &common.ValidationContext{Path: "$"}
	}

	requireStoredIdentity := mode == meta.ValidationModePersistence || mode == meta.ValidationModeResponse
	var result *multierror.Error
	if err := meta.ValidateResource(
		k.TypeMeta,
		k.Metadata,
		meta.ValidationOptions{
			Mode:               mode,
			Path:               vc,
			ExpectedAPIVersion: meta.APIVersionV1Alpha1,
			ExpectedKind:       KeyKind,
			RequireID:          requireStoredIdentity,
			RequireName:        requireStoredIdentity,
			RequireNamespace:   true,
			IDValidator:        ValidateID,
			NamespaceValidator: nschema.ValidatePath,
		},
	); err != nil {
		result = multierror.Append(result, err)
	}

	if k.Metadata.Generation != 0 {
		result = multierror.Append(result, vc.NewErrorForField("metadata.generation", "does not apply to keys"))
	}
	if k.Spec.Usage != "" && !IsValidUsage(k.Spec.Usage) {
		result = multierror.Append(result, vc.NewErrorForField("spec.usage", "is not a recognized key usage"))
	}
	if k.Spec.MaterialType != "" && !IsValidMaterialType(k.Spec.MaterialType) {
		result = multierror.Append(result, vc.NewErrorForField("spec.materialType", "is not a recognized key material type"))
	}
	if k.Spec.DesiredState != "" && !IsValidState(k.Spec.DesiredState) {
		result = multierror.Append(result, vc.NewErrorForField("spec.desiredState", "is not a recognized key state"))
	}
	if k.Spec.KeyData != nil && k.Spec.KeyData.InnerVal == nil {
		result = multierror.Append(result, vc.NewErrorForField("spec.keyData", "must contain provider configuration"))
	}
	if (mode == meta.ValidationModeCreate || mode == meta.ValidationModeConfig) && k.Spec.KeyData == nil {
		result = multierror.Append(result, vc.NewErrorForField("spec.keyData", "is required"))
	}
	if requireStoredIdentity {
		if k.Spec.Usage == "" {
			result = multierror.Append(result, vc.NewErrorForField("spec.usage", "is required"))
		}
		if k.Spec.MaterialType == "" {
			result = multierror.Append(result, vc.NewErrorForField("spec.materialType", "is required"))
		}
		if k.Spec.DesiredState == "" {
			result = multierror.Append(result, vc.NewErrorForField("spec.desiredState", "is required"))
		}
	}

	if err := meta.ValidateStatus(k.Status, mode, vc); err != nil {
		result = multierror.Append(result, err)
	}
	if k.Status != nil && !IsValidState(k.Status.State) {
		result = multierror.Append(result, vc.NewErrorForField("status.state", "is not a recognized key state"))
	}
	if requireStoredIdentity && k.Status == nil {
		result = multierror.Append(result, vc.NewErrorForField("status", "is required"))
	}

	return result.ErrorOrNil()
}

// ValidateFor validates a partial managed Key at the API update boundary.
func (p *KeyPatch) ValidateFor(mode meta.ValidationMode, vc *common.ValidationContext) error {
	if p == nil {
		return fmt.Errorf("key patch is required")
	}
	if vc == nil {
		vc = &common.ValidationContext{Path: "$"}
	}

	var result *multierror.Error
	if err := meta.ValidateTypeMeta(p.TypeMeta, meta.APIVersionV1Alpha1, KeyKind, vc); err != nil {
		result = multierror.Append(result, err)
	}
	if p.Metadata == nil {
		result = multierror.Append(result, vc.NewErrorForField("metadata", "is required and must not be null"))
	} else if err := meta.ValidateObjectMetaPatch(*p.Metadata, meta.ValidationOptions{
		Mode:               mode,
		Path:               vc,
		IDValidator:        ValidateID,
		NamespaceValidator: nschema.ValidatePath,
	}); err != nil {
		result = multierror.Append(result, err)
	}
	if p.Spec == nil {
		result = multierror.Append(result, vc.NewErrorForField("spec", "is required and must not be null"))
	} else {
		if p.Spec.Usage != nil && !IsValidUsage(*p.Spec.Usage) {
			result = multierror.Append(result, vc.NewErrorForField("spec.usage", "is not a recognized key usage"))
		}
		if p.Spec.MaterialType != nil && !IsValidMaterialType(*p.Spec.MaterialType) {
			result = multierror.Append(result, vc.NewErrorForField("spec.materialType", "is not a recognized key material type"))
		}
		if p.Spec.DesiredState != nil && !IsValidState(*p.Spec.DesiredState) {
			result = multierror.Append(result, vc.NewErrorForField("spec.desiredState", "is not a recognized key state"))
		}
		if p.Spec.HasKeyData() && (p.Spec.KeyData == nil || p.Spec.KeyData.InnerVal == nil) {
			result = multierror.Append(result, vc.NewErrorForField("spec.keyData", "must not be null or empty"))
		}
	}
	if err := meta.ValidateStatus(p.Status, mode, vc); err != nil {
		result = multierror.Append(result, err)
	}
	return result.ErrorOrNil()
}

// ApplyTo applies p to a copy of current and verifies immutable key policy and
// identity have not changed.
func (p *KeyPatch) ApplyTo(current *Key, vc *common.ValidationContext) (*Key, error) {
	if current == nil {
		return nil, fmt.Errorf("current key is required")
	}
	if err := p.ValidateFor(meta.ValidationModeUpdate, vc); err != nil {
		return nil, err
	}

	updated, err := current.Clone()
	if err != nil {
		return nil, err
	}
	updated.Metadata = meta.ApplyObjectMetaPatch(updated.Metadata, *p.Metadata)
	if p.Spec.Usage != nil {
		updated.Spec.Usage = *p.Spec.Usage
	}
	if p.Spec.MaterialType != nil {
		updated.Spec.MaterialType = *p.Spec.MaterialType
	}
	if p.Spec.DesiredState != nil {
		updated.Spec.DesiredState = *p.Spec.DesiredState
	}
	if p.Spec.HasKeyData() {
		updated.Spec.KeyData = p.Spec.KeyData
	}

	if err := ValidateUpdate(current, updated, vc); err != nil {
		return nil, err
	}
	return updated, nil
}

// ValidateUpdate rejects changes to immutable key identity and policy.
func ValidateUpdate(before, after *Key, vc *common.ValidationContext) error {
	if before == nil || after == nil {
		return fmt.Errorf("before and after keys are required")
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
	if before.Spec.Usage != after.Spec.Usage {
		result = multierror.Append(result, vc.NewErrorForField("spec.usage", "is immutable"))
	}
	if before.Spec.MaterialType != after.Spec.MaterialType {
		result = multierror.Append(result, vc.NewErrorForField("spec.materialType", "is immutable"))
	}
	return result.ErrorOrNil()
}

// ValidateID verifies an immutable managed-key ID.
func ValidateID(value string) error {
	id, err := apid.Parse(value)
	if err != nil {
		return err
	}
	if id.Prefix() != apid.PrefixKey {
		return fmt.Errorf("must be a key id")
	}
	return nil
}

func IsValidUsage(value KeyUsage) bool {
	return value == KeyUsageDataEncryption
}

func IsValidMaterialType(value KeyMaterialType) bool {
	switch value {
	case KeyMaterialTypeSymmetric, KeyMaterialTypePublic, KeyMaterialTypePrivate, KeyMaterialTypeExternal:
		return true
	default:
		return false
	}
}

func IsValidState(value KeyState) bool {
	switch value {
	case KeyStateActive, KeyStateDisabled:
		return true
	default:
		return false
	}
}

// LogValue prevents managed provider configuration from being serialized into
// structured logs. Safe identity and policy fields remain available.
func (k Key) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("apiVersion", string(k.APIVersion)),
		slog.String("kind", string(k.Kind)),
		slog.String("id", k.Metadata.ID),
		slog.String("name", string(k.Metadata.Name)),
		slog.String("namespace", k.Metadata.Namespace),
		slog.String("usage", string(k.Spec.Usage)),
		slog.String("materialType", string(k.Spec.MaterialType)),
		slog.String("desiredState", string(k.Spec.DesiredState)),
		slog.Bool("keyDataConfigured", k.Spec.KeyData != nil),
	)
}

// LogValue never emits provider configuration or key material.
func (kd KeyData) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("provider", string(kd.GetProviderType())),
		slog.Bool("configured", kd.InnerVal != nil),
	)
}

// LogValue never emits signing material.
func (k SigningKey) LogValue() slog.Value {
	return slog.StringValue("[REDACTED SIGNING KEY]")
}
