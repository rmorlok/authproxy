package api

import (
	apiv1alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	toolschema "github.com/rmorlok/authproxy/internal/schema/resources/tools"
)

// ListToolsResponseJson is the management list envelope for Tool resources.
// Items are full management resources, including executor definitions, so this
// response requires management `tools:list` access. Agent discovery uses the
// separate usage descriptor contracts instead.
type ListToolsResponseJson struct {
	apiv1alpha1.ResourceList[toolschema.Tool] `json:",inline" yaml:",inline"`
}

// NewListToolsResponseJson wraps one page of Tools in a v1alpha1 ToolList
// envelope. An empty continueToken means there are no more pages.
func NewListToolsResponseJson(items []toolschema.Tool, continueToken string) ListToolsResponseJson {
	return ListToolsResponseJson{
		ResourceList: apiv1alpha1.NewResourceList(
			toolschema.ToolKind,
			items,
			apiv1alpha1.ListMeta{Continue: continueToken},
		),
	}
}
