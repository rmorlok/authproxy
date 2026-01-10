package schema

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_AllSchemasCompile(t *testing.T) {
	for _, schemaId := range allSchemas {
		_, err := CompileSchema(schemaId)
		require.NoError(t, err, "schema %s should compile", schemaId)
	}
}
