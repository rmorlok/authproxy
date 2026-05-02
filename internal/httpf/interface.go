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
	GetLabels() map[string]string
}

// Actor is the minimum surface needed to attach an actor's identity and
// labels to an outgoing request. The full Actor type lives in apauth/core
// (it carries permissions, session info, etc.) but for label-snapshot
// purposes the request-info factory only needs the id, namespace, and
// label set. A nil Actor (e.g. background token-refresh requests where no
// actor initiated the call) is a valid input for ForActor.
type Actor interface {
	GetId() apid.ID
	GetNamespace() string
	GetLabels() map[string]string
}

//go:generate mockgen -source=./interface.go -destination=./mock/httpf.go -package=mock
type F interface {
	New() *gentleman.Client
	ForRequestInfo(ri RequestInfo) F
	ForRequestType(rt RequestType) F
	ForConnectorVersion(cv ConnectorVersion) F
	ForConnection(cv Connection) F
	ForActor(actor Actor) F
	ForLabels(labels map[string]string) F
}
