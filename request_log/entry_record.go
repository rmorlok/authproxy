package request_log

import (
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
)

// EntryRecord represents a record of an HTTP request as is stored in the request log in redis. This
// data is redacted to avoid containing sensitive information like information in headers. For a given
// record in redis, the full request may be stored as well, which would correspond to the data in the
// Entry struct.
//
// JSON tagging on this struct is used so the same data structure can be passed directly to endpoint
// responses. It is not use for internal storage.
type EntryRecord struct {
	Type                RequestType   `json:"type"`
	RequestId           uuid.UUID     `json:"request_id"`
	CorrelationId       string        `json:"correlation_id,omitempty"`
	Timestamp           time.Time     `json:"timestamp"`
	Duration            time.Duration `json:"duration"`
	ConnectionId        uuid.UUID     `json:"connection_id,omitempty"`
	ConnectorType       string        `json:"connector_type,omitempty"`
	ConnectorId         uuid.UUID     `json:"connector_id,omitempty"`
	ConnectorVersion    uint64        `json:"connector_version,omitempty"`
	Method              string        `json:"method"`
	Host                string        `json:"host"`
	Scheme              string        `json:"scheme"`
	Path                string        `json:"path"`
	ResponseStatusCode  int           `json:"response_status_code,omitempty"`
	ResponseError       string        `json:"response_error,omitempty"`
	RequestHttpVersion  string        `json:"request_http_version,omitempty"`
	RequestSizeBytes    int64         `json:"request_size_bytes,omitempty"`
	RequestMimeType     string        `json:"request_mime_type,omitempty"`
	ResponseHttpVersion string        `json:"response_http_version,omitempty"`
	ResponseSizeBytes   int64         `json:"response_size_bytes,omitempty"`
	ResponseMimeType    string        `json:"response_mime_type,omitempty"`
}

func (e *EntryRecord) setRedisRecordFields(vals map[string]interface{}) {
	vals[fieldType] = string(e.Type)
	vals[fieldRequestId] = e.RequestId.String()
	if e.CorrelationId == "" {
		vals[fieldCorrelationId] = e.CorrelationId
	}
	vals[fieldTimestamp] = e.Timestamp.UnixMilli()
	vals[fieldDurationMs] = int64(e.Duration)
	if e.ConnectionId != uuid.Nil {
		vals[fieldConnectionId] = e.ConnectionId.String()
	}
	if e.ConnectorType != "" {
		vals[fieldConnectorType] = e.ConnectorType
	}
	if e.ConnectorId != uuid.Nil {
		vals[fieldConnectorId] = e.ConnectorId.String()
		vals[fieldConnectorVersion] = e.ConnectorVersion
	}
	vals[fieldMethod] = e.Method
	vals[fieldHost] = e.Host
	vals[fieldScheme] = e.Scheme
	vals[fieldPath] = e.Path
	if e.ResponseStatusCode == 0 {
		vals[fieldResponseStatusCode] = e.ResponseStatusCode
	}
	if e.ResponseError != "" {
		vals[fieldResponseError] = e.ResponseError
	}
	if e.RequestHttpVersion != "" {
		vals[fieldRequestHttpVersion] = e.RequestHttpVersion
	}
	vals[fieldRequestSizeBytes] = e.RequestSizeBytes
	if e.RequestMimeType != "" {
		vals[fieldRequestMimeType] = e.RequestMimeType
	}
	if e.ResponseHttpVersion != "" {
		vals[fieldResponseHttpVersion] = e.ResponseHttpVersion
	}
	vals[fieldResponseSizeBytes] = e.ResponseSizeBytes
	if e.ResponseMimeType != "" {
		vals[fieldResponseMimeType] = e.ResponseMimeType
	}
}

// EntryRecordFromRedisFields creates an EntryRecord from the redis fields. Note that the fields are a string/string
// map because that is what comes back from the go-redis client for RESP2 protocol.
func EntryRecordFromRedisFields(vals map[string]string) (*EntryRecord, error) {
	if vals == nil {
		return nil, nil
	}

	var err error
	er := &EntryRecord{}
	er.Type = RequestType(vals[fieldType])

	if er.RequestId, err = uuid.Parse(vals[fieldRequestId]); err != nil {
		return nil, errors.Wrap(err, "failed to parse request id")
	}

	er.CorrelationId = vals[fieldCorrelationId]

	timestampMillis, err := strconv.ParseInt(vals[fieldTimestamp], 10, 64)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse timestamp")
	}
	er.Timestamp = time.Unix(0, timestampMillis*int64(time.Millisecond))

	if er.Duration, err = time.ParseDuration(vals[fieldDurationMs] + "ms"); err != nil {
		return nil, errors.Wrap(err, "failed to parse duration")
	}

	if er.ConnectionId, err = uuid.Parse(vals[fieldConnectionId]); err != nil {
		return nil, errors.Wrap(err, "failed to parse connection id")
	}

	er.ConnectorType = vals[fieldConnectorType]

	if er.ConnectorId, err = uuid.Parse(vals[fieldConnectorId]); err != nil {
		return nil, errors.Wrap(err, "failed to parse connector id")
	}

	if er.ConnectorVersion, err = strconv.ParseUint(vals[fieldConnectorVersion], 10, 64); err != nil {
		return nil, errors.Wrap(err, "failed to parse connector version")
	}

	er.Method = vals[fieldMethod]
	er.Host = vals[fieldHost]
	er.Scheme = vals[fieldScheme]
	er.Path = vals[fieldPath]

	if er.ResponseStatusCode, err = strconv.Atoi(vals[fieldResponseStatusCode]); err != nil {
		return nil, errors.Wrap(err, "failed to parse response status code")
	}

	er.ResponseError = vals[fieldResponseError]
	er.RequestHttpVersion = vals[fieldRequestHttpVersion]

	if er.RequestSizeBytes, err = strconv.ParseInt(vals[fieldRequestSizeBytes], 10, 64); err != nil {
		return nil, errors.Wrap(err, "failed to parse request size bytes")
	}

	er.RequestMimeType = vals[fieldRequestMimeType]
	er.ResponseHttpVersion = vals[fieldResponseHttpVersion]

	if er.ResponseSizeBytes, err = strconv.ParseInt(vals[fieldResponseSizeBytes], 10, 64); err != nil {
		return nil, errors.Wrap(err, "failed to parse response size bytes")
	}

	er.ResponseMimeType = vals[fieldResponseMimeType]

	return er, nil
}
