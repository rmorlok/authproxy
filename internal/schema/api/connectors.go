package api

import (
	"github.com/rmorlok/authproxy/internal/apid"
	apiv1alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
)

type ListConnectorsResponseJson struct {
	apiv1alpha1.ResourceList[cschema.Connector] `json:",inline" yaml:",inline"`
}

func NewListConnectorsResponseJson(
	items []cschema.Connector,
	continueToken string,
) ListConnectorsResponseJson {
	return ListConnectorsResponseJson{
		ResourceList: apiv1alpha1.NewResourceList(
			cschema.ConnectorKind,
			items,
			apiv1alpha1.ListMeta{Continue: continueToken},
		),
	}
}

type ListConnectorVersionsResponseJson struct {
	apiv1alpha1.ResourceList[cschema.Connector] `json:",inline" yaml:",inline"`
}

func NewListConnectorVersionsResponseJson(
	items []cschema.Connector,
	continueToken string,
) ListConnectorVersionsResponseJson {
	return ListConnectorVersionsResponseJson{
		ResourceList: apiv1alpha1.NewResourceList(
			cschema.ConnectorKind,
			items,
			apiv1alpha1.ListMeta{Continue: continueToken},
		),
	}
}

// ConnectorLifecycleRequestJson is the request body for connector-level
// lifecycle operations that run asynchronously.
//
//	@Description	Request to run a connector lifecycle operation
type ConnectorLifecycleRequestJson struct {
	TimeoutSeconds *int64 `json:"timeoutSeconds,omitempty" yaml:"timeoutSeconds,omitempty" example:"600"`
}

// ConnectorLifecycleResponseJson is returned after starting a connector-level
// lifecycle workflow.
//
//	@Description	Response for connector lifecycle operation
type ConnectorLifecycleResponseJson struct {
	TaskId      string  `json:"taskId" yaml:"taskId"`
	ConnectorId apid.ID `json:"connectorId" yaml:"connectorId" swaggertype:"string" example:"cxr_test550e8400abcde"`
}

// ForceConnectorVersionStateRequestJson is the request body for
// PUT /connectors/:id/versions/:version/_force_state.
//
//	@Description	Request to force a connector version state
type ForceConnectorVersionStateRequestJson struct {
	State string `json:"state" yaml:"state" example:"primary"`
}
