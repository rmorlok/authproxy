package api

import (
	apiv1alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	nschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
)

// ListNamespacesResponseJson is the Kubernetes-style namespace list response.
type ListNamespacesResponseJson struct {
	apiv1alpha1.ResourceList[nschema.Namespace] `json:",inline" yaml:",inline"`
}

func NewListNamespacesResponseJson(
	items []nschema.Namespace,
	continueToken string,
) ListNamespacesResponseJson {
	return ListNamespacesResponseJson{
		ResourceList: apiv1alpha1.NewResourceList(
			nschema.NamespaceKind,
			items,
			apiv1alpha1.ListMeta{Continue: continueToken},
		),
	}
}
