package httpf

import (
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/request_log"
	"gopkg.in/h2non/gentleman.v2"
)

type ConnectorVersion interface {
	GetID() uuid.UUID
	GetNamespace() string
	GetVersion() uint64
	GetType() string
}

type Connection interface {
	GetID() uuid.UUID
	GetNamespace() string
	GetConnectorId() uuid.UUID
	GetConnectorVersion() uint64
}

//go:generate mockgen -source=./interface.go -destination=./mock/httpf.go -package=mock
type F interface {
	New() *gentleman.Client
	ForRequestInfo(ri request_log.RequestInfo) F
	ForRequestType(rt request_log.RequestType) F
	ForConnectorVersion(cv ConnectorVersion) F
	ForConnection(cv Connection) F
}
