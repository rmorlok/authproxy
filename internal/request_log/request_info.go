package request_log

import "github.com/rmorlok/authproxy/internal/httpf"

func SetLogRecordFieldsFromRequestInfo(er *LogRecord, ri httpf.RequestInfo) {
	t := ri.Type
	if t == "" {
		t = httpf.RequestTypeGlobal
	}

	er.Namespace = ri.Namespace
	er.Type = t
	er.ConnectorId = ri.ConnectorId
	er.ConnectorVersion = ri.ConnectorVersion
	er.ConnectionId = ri.ConnectionId
}
