package namespace

import (
	"fmt"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
)

const (
	NamespaceKind     meta.Kind = "Namespace"
	EncryptionKeyKind meta.Kind = "Key"
)

// NamespaceState is the server-observed lifecycle state of a namespace.
type NamespaceState string

const (
	NamespaceStateActive     NamespaceState = "active"
	NamespaceStateDestroying NamespaceState = "destroying"
	NamespaceStateDestroyed  NamespaceState = "destroyed"
)

// Namespace is the Kubernetes-style representation of an AuthProxy namespace.
// Metadata.ID is the immutable canonical path, Metadata.Name is its final
// segment, and Metadata.Namespace is the parent path (omitted for root).
type Namespace struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      meta.ObjectMeta  `json:"metadata" yaml:"metadata"`
	Spec          NamespaceSpec    `json:"spec" yaml:"spec"`
	Status        *NamespaceStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

// NamespacePatch is a partial Namespace resource used for updates. Pointer
// fields in Metadata preserve the distinction between an omitted map and an
// explicitly empty map that clears existing values.
type NamespacePatch struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      meta.ObjectMetaPatch `json:"metadata" yaml:"metadata"`
	Spec          NamespaceSpec        `json:"spec" yaml:"spec"`
	Status        *NamespaceStatus     `json:"status,omitempty" yaml:"status,omitempty"`
}

// NamespaceSpec contains desired namespace configuration.
type NamespaceSpec struct {
	EncryptionKeyRef *meta.ObjectReference `json:"encryptionKeyRef,omitempty" yaml:"encryptionKeyRef,omitempty"`
}

// NamespaceStatus contains server-observed namespace state.
type NamespaceStatus struct {
	State NamespaceState `json:"state" yaml:"state"`
}

// NewNamespace returns an empty Namespace with the registered type metadata.
func NewNamespace() *Namespace {
	return &Namespace{TypeMeta: meta.NewTypeMeta(NamespaceKind)}
}

// NewNamespaceResourceForPath returns a Namespace with canonical identity
// metadata for path. Callers populate desired spec and server-owned status.
func NewNamespaceResourceForPath(path string) (*Namespace, error) {
	metadata, err := NewResourceMetadata(path)
	if err != nil {
		return nil, err
	}
	return &Namespace{
		TypeMeta: meta.NewTypeMeta(NamespaceKind),
		Metadata: metadata,
		Spec:     NamespaceSpec{},
	}, nil
}

// NewNamespacePatch returns an empty Namespace update envelope with the
// registered type metadata.
func NewNamespacePatch() *NamespacePatch {
	return &NamespacePatch{TypeMeta: meta.NewTypeMeta(NamespaceKind)}
}

// NewNamespaceForPath returns a create-ready namespace resource derived from
// path. Its server-owned metadata.id is intentionally left empty.
func NewNamespaceForPath(path string) (*Namespace, error) {
	resource, err := NewNamespaceResourceForPath(path)
	if err != nil {
		return nil, err
	}
	resource.Metadata.ID = ""
	return resource, nil
}

// NewEncryptionKeyReference returns a canonical reference to a Key resource.
func NewEncryptionKeyReference(id apid.ID) *meta.ObjectReference {
	return &meta.ObjectReference{
		APIVersion: meta.APIVersionV1Alpha1,
		Kind:       EncryptionKeyKind,
		ID:         id.String(),
	}
}

// EncryptionKeyID parses and type-checks a Namespace encryption-key
// reference. A nil reference means no key was selected.
func EncryptionKeyID(ref *meta.ObjectReference) (*apid.ID, error) {
	if ref == nil {
		return nil, nil
	}
	id, err := apid.Parse(ref.ID)
	if err != nil {
		return nil, err
	}
	if id.Prefix() != apid.PrefixKey {
		return nil, fmt.Errorf("must be a key id")
	}
	return &id, nil
}

// NewResourceMetadata returns the identity metadata derived from path. The
// caller remains responsible for adding labels, annotations, and timestamps.
func NewResourceMetadata(path string) (meta.ObjectMeta, error) {
	if err := ValidatePath(path); err != nil {
		return meta.ObjectMeta{}, err
	}

	result := meta.ObjectMeta{
		ID:   path,
		Name: NameFromPath(path),
	}
	if path != Root {
		result.Namespace = ParentPath(path)
	}
	return result, nil
}

// PathFromMetadata derives a namespace's canonical path from its name and
// parent metadata. Root is represented by name `root` with no parent.
func PathFromMetadata(metadata meta.ObjectMeta) (string, error) {
	if metadata.Name == "" {
		return "", fmt.Errorf("name is required")
	}
	if err := ValidateName(string(metadata.Name)); err != nil {
		return "", err
	}

	if metadata.Namespace == "" {
		if metadata.Name != common.ResourceName(Root) {
			return "", fmt.Errorf("parent namespace is required for non-root namespaces")
		}
		return Root, nil
	}
	if err := ValidatePath(metadata.Namespace); err != nil {
		return "", fmt.Errorf("invalid parent namespace: %w", err)
	}

	return metadata.Namespace + PathSeparator + string(metadata.Name), nil
}

// Clone returns a deep copy of n.
func (n *Namespace) Clone() *Namespace {
	if n == nil {
		return nil
	}
	clone := *n
	clone.Metadata = meta.CloneObjectMeta(n.Metadata)
	if n.Spec.EncryptionKeyRef != nil {
		keyRef := *n.Spec.EncryptionKeyRef
		clone.Spec.EncryptionKeyRef = &keyRef
	}
	if n.Status != nil {
		status := *n.Status
		clone.Status = &status
	}
	return &clone
}

// Validate applies configuration-file resource rules. API handlers use
// ValidateFor with the lifecycle mode appropriate to the request.
func (n *Namespace) Validate(vc *common.ValidationContext) error {
	return n.ValidateFor(meta.ValidationModeConfig, vc)
}

// ValidateFor validates n for one resource lifecycle boundary.
func (n *Namespace) ValidateFor(mode meta.ValidationMode, vc *common.ValidationContext) error {
	if n == nil {
		return fmt.Errorf("namespace is required")
	}
	if vc == nil {
		vc = &common.ValidationContext{Path: "$"}
	}

	requireIdentity := mode == meta.ValidationModeCreate ||
		mode == meta.ValidationModeConfig ||
		mode == meta.ValidationModePersistence ||
		mode == meta.ValidationModeResponse
	requireID := mode == meta.ValidationModePersistence || mode == meta.ValidationModeResponse

	var result *multierror.Error
	if err := meta.ValidateResource(
		n.TypeMeta,
		n.Metadata,
		meta.ValidationOptions{
			Mode:               mode,
			Path:               vc,
			ExpectedAPIVersion: meta.APIVersionV1Alpha1,
			ExpectedKind:       NamespaceKind,
			RequireID:          requireID,
			RequireName:        requireIdentity,
			IDValidator:        ValidatePath,
			NamespaceValidator: ValidatePath,
		},
	); err != nil {
		result = multierror.Append(result, err)
	}

	if n.Metadata.Generation != 0 {
		result = multierror.Append(result, vc.NewErrorForField("metadata.generation", "does not apply to namespaces"))
	}

	if n.Metadata.Name != "" {
		if err := ValidateName(string(n.Metadata.Name)); err != nil {
			result = multierror.Append(result, vc.NewErrorfForField("metadata.name", "%v", err))
		}
	}

	if requireIdentity {
		path, err := PathFromMetadata(n.Metadata)
		if err != nil {
			result = multierror.Append(result, vc.NewErrorfForField("metadata", "%v", err))
		} else if n.Metadata.ID != "" && n.Metadata.ID != path {
			result = multierror.Append(result, vc.NewErrorfForField("metadata.id", "must match path %q derived from metadata.namespace and metadata.name", path))
		}
	}

	if err := validateEncryptionKeyReference(n.Spec.EncryptionKeyRef, vc); err != nil {
		result = multierror.Append(result, err)
	}

	if err := meta.ValidateStatus(n.Status, mode, vc); err != nil {
		result = multierror.Append(result, err)
	}
	if n.Status != nil && !IsValidState(n.Status.State) {
		result = multierror.Append(result, vc.NewErrorForField("status.state", "is not a recognized namespace state"))
	}
	if (mode == meta.ValidationModePersistence || mode == meta.ValidationModeResponse) && n.Status == nil {
		result = multierror.Append(result, vc.NewErrorForField("status", "is required"))
	}

	return result.ErrorOrNil()
}

// ValidateFor validates a partial Namespace at the API update boundary.
func (p *NamespacePatch) ValidateFor(mode meta.ValidationMode, vc *common.ValidationContext) error {
	if p == nil {
		return fmt.Errorf("namespace patch is required")
	}
	if vc == nil {
		vc = &common.ValidationContext{Path: "$"}
	}

	var result *multierror.Error
	if err := meta.ValidateTypeMeta(p.TypeMeta, meta.APIVersionV1Alpha1, NamespaceKind, vc); err != nil {
		result = multierror.Append(result, err)
	}
	if err := meta.ValidateObjectMetaPatch(p.Metadata, meta.ValidationOptions{
		Mode:               mode,
		Path:               vc,
		IDValidator:        ValidatePath,
		NamespaceValidator: ValidatePath,
	}); err != nil {
		result = multierror.Append(result, err)
	}
	if p.Metadata.Name != nil {
		if err := ValidateName(string(*p.Metadata.Name)); err != nil {
			result = multierror.Append(result, vc.NewErrorfForField("metadata.name", "%v", err))
		}
	}
	if err := validateEncryptionKeyReference(p.Spec.EncryptionKeyRef, vc); err != nil {
		result = multierror.Append(result, err)
	}
	if err := meta.ValidateStatus(p.Status, mode, vc); err != nil {
		result = multierror.Append(result, err)
	}

	return result.ErrorOrNil()
}

// ApplyTo applies p to a clone of current and verifies that immutable
// namespace identity is unchanged.
func (p *NamespacePatch) ApplyTo(current *Namespace, vc *common.ValidationContext) (*Namespace, error) {
	if current == nil {
		return nil, fmt.Errorf("current namespace is required")
	}
	if err := p.ValidateFor(meta.ValidationModeUpdate, vc); err != nil {
		return nil, err
	}

	updated := current.Clone()
	updated.Metadata = meta.ApplyObjectMetaPatch(updated.Metadata, p.Metadata)
	if p.Spec.EncryptionKeyRef != nil {
		keyRef := *p.Spec.EncryptionKeyRef
		updated.Spec.EncryptionKeyRef = &keyRef
	}
	if err := ValidateUpdate(current, updated, vc); err != nil {
		return nil, err
	}
	return updated, nil
}

func validateEncryptionKeyReference(ref *meta.ObjectReference, vc *common.ValidationContext) error {
	if ref == nil {
		return nil
	}
	if vc == nil {
		vc = &common.ValidationContext{Path: "$"}
	}

	refPath := vc.PushField("spec").PushField("encryptionKeyRef")
	var result *multierror.Error
	if err := meta.ValidateObjectReference(*ref, refPath); err != nil {
		result = multierror.Append(result, err)
	}
	if ref.APIVersion != meta.APIVersionV1Alpha1 {
		result = multierror.Append(result, refPath.NewErrorfForField("apiVersion", "must be %q", meta.APIVersionV1Alpha1))
	}
	if ref.Kind != EncryptionKeyKind {
		result = multierror.Append(result, refPath.NewErrorfForField("kind", "must be %q", EncryptionKeyKind))
	}
	if ref.ID == "" {
		result = multierror.Append(result, refPath.NewErrorForField("id", "is required"))
	} else if _, err := EncryptionKeyID(ref); err != nil {
		result = multierror.Append(result, refPath.NewErrorfForField("id", "%v", err))
	}
	return result.ErrorOrNil()
}

// ValidateUpdate rejects changes to namespace identity after applying a full
// or partial update resource to an existing namespace.
func ValidateUpdate(before, after *Namespace, vc *common.ValidationContext) error {
	if before == nil || after == nil {
		return fmt.Errorf("before and after namespaces are required")
	}
	if vc == nil {
		vc = &common.ValidationContext{Path: "$"}
	}

	var result *multierror.Error
	if err := meta.ValidateTypeMetaUpdate(before.TypeMeta, after.TypeMeta, vc); err != nil {
		result = multierror.Append(result, err)
	}
	if err := meta.ValidateMetadataUpdate(before.Metadata, after.Metadata, meta.UpdateOptions{
		ImmutableName:      true,
		ImmutableNamespace: true,
	}, vc); err != nil {
		result = multierror.Append(result, err)
	}
	return result.ErrorOrNil()
}

// IsValidState reports whether state is a recognized namespace lifecycle
// value.
func IsValidState(state NamespaceState) bool {
	switch state {
	case NamespaceStateActive, NamespaceStateDestroying, NamespaceStateDestroyed:
		return true
	default:
		return false
	}
}
