package httpf

import (
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/connectors"
)

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
	ConnectorId      apid.ID
	ConnectorVersion uint64
	ConnectionId     apid.ID

	// RateLimiting is the rate limiting configuration for the connector, if available.
	// Nil means use default behavior (enabled with standard Retry-After parsing).
	RateLimiting *connectors.RateLimiting
}
