package api

import (
	apiv1alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	keyschema "github.com/rmorlok/authproxy/internal/schema/resources/key"
)

type ListKeysResponseJson struct {
	apiv1alpha1.ResourceList[keyschema.Key] `json:",inline" yaml:",inline"`
}

func NewListKeysResponseJson(
	items []keyschema.Key,
	continueToken string,
) ListKeysResponseJson {
	return ListKeysResponseJson{
		ResourceList: apiv1alpha1.NewResourceList(
			keyschema.KeyKind,
			items,
			apiv1alpha1.ListMeta{Continue: continueToken},
		),
	}
}
