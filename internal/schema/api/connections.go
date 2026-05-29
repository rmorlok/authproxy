package api

import (
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
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
	Labels      map[string]string     `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations map[string]string     `json:"annotations,omitempty" yaml:"annotations,omitempty"`
	State       ConnectionState       `json:"state" yaml:"state" swaggertype:"string" example:"configured"`
	HealthState ConnectionHealthState `json:"health_state" yaml:"health_state" swaggertype:"string" example:"healthy"`
	SetupStep   *cschema.SetupStep    `json:"setup_step_id,omitempty" yaml:"setup_step_id,omitempty" swaggertype:"string" example:"tenant"`
	SetupError  *string               `json:"setup_error,omitempty" yaml:"setup_error,omitempty"`
	Connector   ConnectorJson         `json:"connector" yaml:"connector"`
	CreatedAt   time.Time             `json:"created_at" yaml:"created_at"`
	UpdatedAt   time.Time             `json:"updated_at" yaml:"updated_at"`
}

type ListConnectionResponseJson struct {
	Items  []ConnectionJson `json:"items" yaml:"items"`
	Cursor string           `json:"cursor,omitempty" yaml:"cursor,omitempty"`
}

type DisconnectResponseJson struct {
	TaskId     string         `json:"task_id" yaml:"task_id"`
	Connection ConnectionJson `json:"connection" yaml:"connection"`
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
	Labels      map[string]string `json:"labels" yaml:"labels"`
	Annotations map[string]string `json:"annotations" yaml:"annotations"`
}

// ProxyResponseJson is the response from a proxied request.
//
//	@Description	Response from a proxied HTTP request
type ProxyResponseJson struct {
	StatusCode int               `json:"status_code" yaml:"status_code" example:"200"`
	Headers    map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
	BodyRaw    []byte            `json:"body_raw,omitempty" yaml:"body_raw,omitempty"`
	BodyJson   interface{}       `json:"body_json,omitempty" yaml:"body_json,omitempty"`
}
