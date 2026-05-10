package request_log

import (
	"context"

	"github.com/rmorlok/authproxy/internal/httpf"
)

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
	er.Labels = ri.Labels
}

// ApplyAttributionToLogRecord stamps the LogRecord with whatever the
// proxy stack recorded on the request context — the response source
// plus, when a rate-limit resource matched, the rule id / mode / bucket
// / full match set. When no attribution was installed (older code paths
// or non-proxy traffic), defaults the source to ResponseSourceUpstream
// so the column is always populated.
//
// Designed as a separate call from SetLogRecordFieldsFromRequestInfo so
// the existing signature isn't disturbed; the request-log round-tripper
// invokes both in sequence.
func ApplyAttributionToLogRecord(er *LogRecord, ctx context.Context) {
	attr := AttributionFromContext(ctx)
	if attr == nil {
		if er.ResponseSource == "" {
			er.ResponseSource = ResponseSourceUpstream
		}
		return
	}

	if attr.Source != "" {
		er.ResponseSource = attr.Source
	} else if er.ResponseSource == "" {
		er.ResponseSource = ResponseSourceUpstream
	}

	if !attr.RateLimitId.IsNil() {
		er.RateLimitId = attr.RateLimitId
	}
	if attr.RateLimitMode != "" {
		er.RateLimitMode = attr.RateLimitMode
	}
	if len(attr.RateLimitBucket) > 0 {
		er.RateLimitBucket = attr.RateLimitBucket
	}
	if len(attr.RateLimitMatched) > 0 {
		er.RateLimitMatched = attr.RateLimitMatched
	}
}
