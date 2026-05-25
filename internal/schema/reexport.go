package schema

import (
	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
)

const SchemaIdAPI = api.SchemaIdAPI
const SchemaIdAuth = auth.SchemaIdAuth
const SchemaIdCommon = common.SchemaIdCommon
const SchemaIdConfig = config.SchemaIdConfig
const SchemaIdConnectors = connectors.SchemaIdConnectors
const SchemaIdNamespace = namespace.SchemaIdNamespace
const SchemaIdRateLimit = rate_limit.SchemaIdRateLimit

var allSchemas = []string{
	SchemaIdAPI,
	SchemaIdAuth,
	SchemaIdCommon,
	SchemaIdConfig,
	SchemaIdConnectors,
	SchemaIdNamespace,
	SchemaIdRateLimit,
}
