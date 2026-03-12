package httpf

import (
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/connectors"
	"gopkg.in/h2non/gentleman.v2"
)

type ConnectorVersion interface {
	GetId() apid.ID
	GetNamespace() string
	GetVersion() uint64
}

type GettableConnectorVersion interface {
	GetConnectorVersionEntity() ConnectorVersion
}

// RateLimitConfigProvider is an optional interface that connections can implement to provide
// rate limiting configuration from the connector definition.
type RateLimitConfigProvider interface {
	GetRateLimitConfig() *connectors.RateLimiting
}

type Connection interface {
	GetId() apid.ID
	GetNamespace() string
	GetConnectorId() apid.ID
	GetConnectorVersion() uint64
}

//go:generate mockgen -source=./interface.go -destination=./mock/httpf.go -package=mock
type F interface {
	New() *gentleman.Client
	ForRequestInfo(ri RequestInfo) F
	ForRequestType(rt RequestType) F
	ForConnectorVersion(cv ConnectorVersion) F
	ForConnection(cv Connection) F
}
