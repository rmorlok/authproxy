package api

import (
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	apiv1alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	"github.com/rmorlok/authproxy/internal/schema/common"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
)

// ConnectionState is the API-visible lifecycle state of a connection.
type ConnectionState string

const (
	ConnectionStateSetup         ConnectionState = "setup"
	ConnectionStateConfigured    ConnectionState = "configured"
	ConnectionStateDisabled      ConnectionState = "disabled"
	ConnectionStateDisconnecting ConnectionState = "disconnecting"
	ConnectionStateDisconnected  ConnectionState = "disconnected"
)

// ConnectionHealthState is the API-visible operational health signal for a connection.
type ConnectionHealthState string

const (
	ConnectionHealthStateHealthy   ConnectionHealthState = "healthy"
	ConnectionHealthStateUnhealthy ConnectionHealthState = "unhealthy"
)

// ConnectionJson is the API projection of a connection resource.
//
//	@Description	Connection to an external service
type ConnectionJson struct {
	Id          apid.ID               `json:"id" yaml:"id" swaggertype:"string" example:"cxn_test550e8400abcde"`
	Namespace   string                `json:"namespace" yaml:"namespace" example:"root.acme"`
	Name        common.ResourceName   `json:"name" yaml:"name" swaggertype:"string" example:"production-crm"`
	Labels      map[string]string     `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations map[string]string     `json:"annotations,omitempty" yaml:"annotations,omitempty"`
	State       ConnectionState       `json:"state" yaml:"state" swaggertype:"string" example:"configured"`
	HealthState ConnectionHealthState `json:"healthState" yaml:"healthState" swaggertype:"string" example:"healthy"`
	SetupStep   *cschema.SetupStep    `json:"setupStepId,omitempty" yaml:"setupStepId,omitempty" swaggertype:"string" example:"tenant"`
	SetupError  *string               `json:"setupError,omitempty" yaml:"setupError,omitempty"`
	Connector   ConnectorJson         `json:"connector" yaml:"connector"`
	CreatedAt   time.Time             `json:"createdAt" yaml:"createdAt"`
	UpdatedAt   time.Time             `json:"updatedAt" yaml:"updatedAt"`
}

type ListConnectionResponseJson = apiv1alpha1.ResourceList[ConnectionJson]

func NewListConnectionResponseJson(items []ConnectionJson, continueToken string) ListConnectionResponseJson {
	return apiv1alpha1.NewResourceList("Connection", items, apiv1alpha1.ListMeta{Continue: continueToken})
}

type DisconnectResponseJson struct {
	TaskId     string         `json:"taskId" yaml:"taskId"`
	Connection ConnectionJson `json:"connection" yaml:"connection"`
}

// MigrateConnectionVersionRequestJson is the request body for POST /connections/:id/_migrate_version.
//
//	@Description	Request to migrate a connection to another connector version
type MigrateConnectionVersionRequestJson struct {
	TargetVersion  uint64 `json:"targetVersion" yaml:"targetVersion" example:"3"`
	TimeoutSeconds *int64 `json:"timeoutSeconds,omitempty" yaml:"timeoutSeconds,omitempty" example:"600"`
}

// MigrateConnectionVersionResponseJson is returned when a connection-version migration workflow starts.
//
//	@Description	Response returned after starting a connection connector-version migration workflow
type MigrateConnectionVersionResponseJson struct {
	TaskId        string  `json:"taskId" yaml:"taskId"`
	ConnectionId  apid.ID `json:"connectionId" yaml:"connectionId" swaggertype:"string" example:"cxn_test550e8400abcde"`
	SourceVersion uint64  `json:"sourceVersion" yaml:"sourceVersion" example:"1"`
	TargetVersion uint64  `json:"targetVersion" yaml:"targetVersion" example:"3"`
}

// DisconnectConnectionRequestJson is the optional request body for POST /connections/:id/_disconnect.
//
//	@Description	Request to disconnect a connection
type DisconnectConnectionRequestJson struct {
	TimeoutSeconds *int64 `json:"timeoutSeconds,omitempty" yaml:"timeoutSeconds,omitempty" example:"600"`
}

// ForceConnectionStateRequestJson is the request body for PUT /connections/:id/_force_state.
//
//	@Description	Request to force a connection state
type ForceConnectionStateRequestJson struct {
	State string `json:"state" yaml:"state" example:"configured"`
}

// UpdateConnectionRequestJson is the request body for PATCH /connections/:id.
//
//	@Description	Request to update a connection's labels and annotations
type UpdateConnectionRequestJson struct {
	Name        *common.ResourceName `json:"name,omitempty" yaml:"name,omitempty" swaggertype:"string" example:"production-crm"`
	Labels      map[string]string    `json:"labels" yaml:"labels"`
	Annotations map[string]string    `json:"annotations" yaml:"annotations"`
}

// ProxyResponseJson is the response from a proxied request.
//
//	@Description	Response from a proxied HTTP request
type ProxyResponseJson struct {
	StatusCode int               `json:"statusCode" yaml:"statusCode" example:"200"`
	Headers    map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
	BodyRaw    []byte            `json:"bodyRaw,omitempty" yaml:"bodyRaw,omitempty"`
	BodyJson   interface{}       `json:"bodyJson,omitempty" yaml:"bodyJson,omitempty"`
}
