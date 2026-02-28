package iface

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httpf"
)

type Connection interface {
	/*
	 * Core fields
	 */

	GetId() uuid.UUID
	GetNamespace() string
	GetState() database.ConnectionState
	GetConnectorId() uuid.UUID
	GetConnectorVersion() uint64
	GetCreatedAt() time.Time
	GetUpdatedAt() time.Time
	GetDeletedAt() *time.Time
	GetLabels() map[string]string

	/*
	 * Nested entities
	 */

	GetConnectorVersionEntity() ConnectorVersion

	/*
	 * Methods
	 */

	SetState(ctx context.Context, state database.ConnectionState) error
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
}
