package iface

import (
	"context"
	"net/http"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apjs"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httpf"
)

type Connection interface {
	/*
	 * Core fields
	 */

	GetId() apid.ID
	GetNamespace() string
	GetState() database.ConnectionState
	GetConnectorId() apid.ID
	GetConnectorVersion() uint64
	GetCreatedAt() time.Time
	GetUpdatedAt() time.Time
	GetDeletedAt() *time.Time
	GetLabels() map[string]string
	GetAnnotations() map[string]string
	GetSetupStep() *string

	/*
	 * Nested entities
	 */

	GetConnectorVersionEntity() ConnectorVersion

	/*
	 * Methods
	 */

	SetState(ctx context.Context, state database.ConnectionState) error
	SetSetupStep(ctx context.Context, setupStep *string) error
	GetConfiguration(ctx context.Context) (map[string]any, error)
	SetConfiguration(ctx context.Context, data map[string]any) error
	GetMustacheContext(ctx context.Context) (map[string]any, error)
	GetProbe(probeId string) (Probe, error)
	GetProbes() []Probe
	ProxyRequest(
		ctx context.Context,
		reqType httpf.RequestType,
		req *ProxyRequest,
	) (*ProxyResponse, error)
	ProxyRequestRaw(
		ctx context.Context,
		reqType httpf.RequestType,
		req *ProxyRequest,
		w http.ResponseWriter,
	) error
	SubmitForm(ctx context.Context, req SubmitConnectionRequest) (InitiateConnectionResponse, error)
	GetCurrentSetupStepResponse(ctx context.Context) (InitiateConnectionResponse, error)
	GetDataSource(ctx context.Context, sourceId string) ([]apjs.DataSourceOption, error)
	Reconfigure(ctx context.Context) (InitiateConnectionResponse, error)
}
