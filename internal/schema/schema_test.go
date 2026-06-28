package schema

import (
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
	"github.com/stretchr/testify/require"
)

var allSchemaIDs = []string{
	api.SchemaIdAPI,
	auth.SchemaIdAuth,
	common.SchemaIdCommon,
	config.SchemaIdConfig,
	connectors.SchemaIdConnectors,
	namespace.SchemaIdNamespace,
	rate_limit.SchemaIdRateLimit,
}

func Test_AllSchemasCompile(t *testing.T) {
	for _, schemaId := range allSchemaIDs {
		_, err := CompileSchema(schemaId)
		require.NoError(t, err, "schema %s should compile", schemaId)
	}
}
