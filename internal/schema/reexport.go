package schema

import (
	"github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/connectors"
)

const SchemaIdAuth = auth.SchemaIdAuth
const SchemaIdCommon = common.SchemaIdCommon
const SchemaIdConfig = config.SchemaIdConfig
const SchemaIdConnectors = connectors.SchemaIdConnectors

var allSchemas = []string{
	SchemaIdAuth,
	SchemaIdCommon,
	SchemaIdConfig,
	SchemaIdConnectors,
}
