package httpf

import (
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/connectors"
)

// RequestType is re-exported from schema/common so callers within the
// runtime layers can keep referring to httpf.RequestType while the canonical
// definition (and its validation) lives alongside the rest of the schema
// types.
type RequestType = common.RequestType

const (
	RequestTypeGlobal              = common.RequestTypeGlobal
	RequestTypeProxy               = common.RequestTypeProxy
	RequestTypeOAuth               = common.RequestTypeOAuth
	RequestTypePublic              = common.RequestTypePublic
	RequestTypeProbe               = common.RequestTypeProbe
	RequestTypeOAuth2TokenExchange = common.RequestTypeOAuth2TokenExchange
	RequestTypeOAuth2Refresh       = common.RequestTypeOAuth2Refresh
	RequestTypeOAuth2Revocation    = common.RequestTypeOAuth2Revocation
)

type RequestInfo struct {
	Namespace        string
	Type             RequestType
	ConnectorId      apid.ID
	ConnectorVersion uint64
	ConnectionId     apid.ID
	Labels           map[string]string

	// RateLimiting is the rate limiting configuration for the connector, if available.
	// Nil means use default behavior (enabled with standard Retry-After parsing).
	RateLimiting *connectors.RateLimiting
}
