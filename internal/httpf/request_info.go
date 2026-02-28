package httpf

import "github.com/google/uuid"

type RequestType string

const (
	RequestTypeGlobal RequestType = "global"
	RequestTypeProxy  RequestType = "proxy"
	RequestTypeOAuth  RequestType = "oauth"
	RequestTypePublic RequestType = "public"
	RequestTypeProbe  RequestType = "probe"
)

type RequestInfo struct {
	Namespace        string
	Type             RequestType
	ConnectorId      uuid.UUID
	ConnectorVersion uint64
	ConnectionId     uuid.UUID
}
