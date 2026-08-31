package api

import (
	apiv1alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	nschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
)

// NamespaceJson is the resource-owned Namespace contract used by the v1 API.
type NamespaceJson = nschema.Namespace

// CreateNamespaceRequestJson uses the Namespace resource envelope. Create
// requests omit server-owned metadata and status.
type CreateNamespaceRequestJson = nschema.Namespace

// UpdateNamespaceRequestJson uses a partial Namespace resource envelope.
// Metadata and spec remain required top-level objects while their patch fields
// retain omitted-versus-present semantics.
type UpdateNamespaceRequestJson = nschema.NamespacePatch

// ListNamespacesResponseJson is the Kubernetes-style namespace list response.
type ListNamespacesResponseJson = apiv1alpha1.ResourceList[nschema.Namespace]

func NewListNamespacesResponseJson(
	items []nschema.Namespace,
	continueToken string,
) ListNamespacesResponseJson {
	return apiv1alpha1.NewResourceList(
		nschema.NamespaceKind,
		items,
		apiv1alpha1.ListMeta{Continue: continueToken},
	)
}

// SetNamespaceKeyRequestJson is a Namespace subresource update. The desired
// key is supplied as spec.encryptionKeyRef.
type SetNamespaceKeyRequestJson = nschema.NamespacePatch

// NamespaceKeyJson returns the Namespace resource whose spec contains the
// selected encryption-key reference.
type NamespaceKeyJson = nschema.Namespace
