package request_log

import "github.com/google/uuid"

type RequestType string

const (
	RequestTypeGlobal RequestType = "global"
	RequestTypeProxy  RequestType = "proxy"
	RequestTypeOAuth  RequestType = "oauth"
	RequestTypePublic RequestType = "public"
)

type RequestInfo struct {
	Type             RequestType
	ConnectorType    string
	ConnectorId      uuid.UUID
	ConnectorVersion uint64
	ConnectionId     uuid.UUID
}

func (r *RequestInfo) setRedisRecordFields(vals map[string]interface{}) {
	t := r.Type
	if t == "" {
		t = RequestTypeGlobal
	}

	vals[fieldType] = string(t)

	if r.ConnectorType != "" {
		vals[fieldConnectorType] = r.ConnectorType
	}

	if r.ConnectorId != uuid.Nil {
		vals[fieldConnectorVersion] = r.ConnectorVersion
		vals[fieldConnectorId] = r.ConnectorId.String()
	}

	if r.ConnectionId != uuid.Nil {
		vals[fieldConnectionId] = r.ConnectionId.String()
	}
}
