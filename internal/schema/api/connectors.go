package api

import (
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	apiv1alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	"github.com/rmorlok/authproxy/internal/schema/common"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
)

// ConnectorVersionState is the API-visible lifecycle state of a connector version.
type ConnectorVersionState string

const (
	ConnectorVersionStateDraft    ConnectorVersionState = "draft"
	ConnectorVersionStatePrimary  ConnectorVersionState = "primary"
	ConnectorVersionStateActive   ConnectorVersionState = "active"
	ConnectorVersionStateArchived ConnectorVersionState = "archived"
)

// ConnectorJson represents the API summary projection of a connector version.
//
//	@Description	Connector API summary response
type ConnectorJson struct {
	Id            apid.ID               `json:"id" yaml:"id" swaggertype:"string" example:"cxr_test550e8400abcde"`
	Version       uint64                `json:"version" yaml:"version" example:"1"`
	Namespace     string                `json:"namespace" yaml:"namespace" example:"root.acme"`
	Name          common.ResourceName   `json:"name" yaml:"name" swaggertype:"string" example:"salesforce"`
	State         ConnectorVersionState `json:"state" yaml:"state" swaggertype:"string" example:"primary"`
	DisplayName   string                `json:"displayName" yaml:"displayName" example:"Salesforce"`
	Highlight     string                `json:"highlight,omitempty" yaml:"highlight,omitempty" example:"CRM platform"`
	Description   string                `json:"description" yaml:"description" example:"Salesforce CRM integration"`
	StatusPageUrl string                `json:"statusPageUrl,omitempty" yaml:"statusPageUrl,omitempty" example:"https://status.salesforce.com"`
	Logo          string                `json:"logo" yaml:"logo" example:"https://example.com/logo.png"`
	HasConfigure  bool                  `json:"hasConfigure" yaml:"hasConfigure" example:"false"`
	Labels        map[string]string     `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations   map[string]string     `json:"annotations,omitempty" yaml:"annotations,omitempty"`
	CreatedAt     time.Time             `json:"createdAt" yaml:"createdAt"`
	UpdatedAt     time.Time             `json:"updatedAt" yaml:"updatedAt"`
}

type ListConnectorsResponseJson struct {
	apiv1alpha1.ResourceList[ConnectorJson] `json:",inline" yaml:",inline"`
}

func NewListConnectorsResponseJson(
	items []ConnectorJson,
	continueToken string,
) ListConnectorsResponseJson {
	return ListConnectorsResponseJson{
		ResourceList: apiv1alpha1.NewResourceList(
			"Connector",
			items,
			apiv1alpha1.ListMeta{Continue: continueToken},
		),
	}
}

// ConnectorVersionJson represents a single connector version returned by the API.
//
//	@Description	Detailed connector version information
type ConnectorVersionJson struct {
	Id          apid.ID                     `json:"id" yaml:"id" swaggertype:"string" example:"cxr_test550e8400abcde"`
	Version     uint64                      `json:"version" yaml:"version" example:"1"`
	Namespace   string                      `json:"namespace" yaml:"namespace" example:"root.acme"`
	Name        common.ResourceName         `json:"name" yaml:"name" swaggertype:"string" example:"salesforce"`
	State       ConnectorVersionState       `json:"state" yaml:"state" swaggertype:"string" example:"primary"`
	Definition  cschema.ConnectorDefinition `json:"definition" yaml:"definition"`
	Labels      map[string]string           `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations map[string]string           `json:"annotations,omitempty" yaml:"annotations,omitempty"`
	CreatedAt   time.Time                   `json:"createdAt" yaml:"createdAt"`
	UpdatedAt   time.Time                   `json:"updatedAt" yaml:"updatedAt"`
}

type ListConnectorVersionsResponseJson struct {
	apiv1alpha1.ResourceList[ConnectorVersionJson] `json:",inline" yaml:",inline"`
}

func NewListConnectorVersionsResponseJson(
	items []ConnectorVersionJson,
	continueToken string,
) ListConnectorVersionsResponseJson {
	return ListConnectorVersionsResponseJson{
		ResourceList: apiv1alpha1.NewResourceList(
			"Connector",
			items,
			apiv1alpha1.ListMeta{Continue: continueToken},
		),
	}
}

// CreateConnectorRequestJson is the request body for POST /connectors.
//
//	@Description	Request to create a new connector
type CreateConnectorRequestJson struct {
	Namespace   string                      `json:"namespace" yaml:"namespace" example:"root.acme"`
	Name        *common.ResourceName        `json:"name,omitempty" yaml:"name,omitempty" swaggertype:"string" example:"salesforce"`
	Definition  cschema.ConnectorDefinition `json:"definition" yaml:"definition"`
	Labels      map[string]string           `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations map[string]string           `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

// UpdateConnectorRequestJson is the request body for PATCH /connectors/:id.
//
//	@Description	Request to update a logical connector
type UpdateConnectorRequestJson struct {
	Name        *common.ResourceName         `json:"name,omitempty" yaml:"name,omitempty" swaggertype:"string" example:"salesforce"`
	Definition  *cschema.ConnectorDefinition `json:"definition,omitempty" yaml:"definition,omitempty"`
	Labels      *map[string]string           `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations *map[string]string           `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

// UpdateConnectorVersionRequestJson is the request body for PATCH /connectors/:id/versions/:version.
// Connector-level fields such as name are intentionally excluded.
//
//	@Description Request to update a connector definition version
type UpdateConnectorVersionRequestJson struct {
	Definition  *cschema.ConnectorDefinition `json:"definition,omitempty" yaml:"definition,omitempty"`
	Labels      *map[string]string           `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations *map[string]string           `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

// CreateConnectorVersionRequestJson is the request body for POST /connectors/:id/versions.
//
//	@Description	Request to create a new draft connector version
type CreateConnectorVersionRequestJson struct {
	Definition  *cschema.ConnectorDefinition `json:"definition,omitempty" yaml:"definition,omitempty"`
	Labels      *map[string]string           `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations *map[string]string           `json:"annotations,omitempty" yaml:"annotations,omitempty"`
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
