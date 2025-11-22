package request_log

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
	ConnectorType    string
	ConnectorId      uuid.UUID
	ConnectorVersion uint64
	ConnectionId     uuid.UUID
}

func (r *RequestInfo) setRedisRecordFields(er *EntryRecord) {
	t := r.Type
	if t == "" {
		t = RequestTypeGlobal
	}

	er.Namespace = r.Namespace
	er.Type = t
	er.ConnectorType = r.ConnectorType
	er.ConnectorId = r.ConnectorId
	er.ConnectorVersion = r.ConnectorVersion
	er.ConnectionId = r.ConnectionId
}
