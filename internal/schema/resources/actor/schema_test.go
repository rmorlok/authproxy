package actor

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	jsonschemav5 "github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/require"
)

type schemaIDEnvelope struct {
	ID string `json:"$id"`
}

func addSchema(t *testing.T, compiler *jsonschemav5.Compiler, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	var envelope schemaIDEnvelope
	require.NoError(t, json.Unmarshal(contents, &envelope))
	require.NoError(t, compiler.AddResource(envelope.ID, bytes.NewReader(contents)))
	return envelope.ID
}

func compileActorSchema(t *testing.T) *jsonschemav5.Schema {
	t.Helper()
	compiler := jsonschemav5.NewCompiler()
	addSchema(t, compiler, "../../common/schema.json")
	addSchema(t, compiler, "../../auth/schema.json")
	addSchema(t, compiler, "../meta/schema.json")
	addSchema(t, compiler, "../namespace/schema.json")
	addSchema(t, compiler, "../key/schema.json")
	id := addSchema(t, compiler, "./schema.json")
	schema, err := compiler.Compile(id)
	require.NoError(t, err)
	return schema
}

func TestSchemaID(t *testing.T) {
	compiler := jsonschemav5.NewCompiler()
	require.Equal(t, SchemaIdActor, addSchema(t, compiler, "./schema.json"))
}

func TestSchema(t *testing.T) {
	schema := compileActorSchema(t)
	tests := []struct {
		name  string
		valid bool
		data  string
	}{
		{
			name:  "minimal create resource",
			valid: true,
			data:  `{"apiVersion":"authproxy.net/v1alpha1","kind":"Actor","metadata":{"namespace":"root.acme"},"spec":{"externalId":"user-123"}}`,
		},
		{
			name:  "configured resource",
			valid: true,
			data:  `{"apiVersion":"authproxy.net/v1alpha1","kind":"Actor","metadata":{"name":"billing","namespace":"root.acme","labels":{"team":"platform"}},"spec":{"externalId":"user-123","permissions":[{"namespace":"root.acme.**","resources":["connections"],"verbs":["read"]}],"signingKey":{"publicKey":{"value":"public"}}}}`,
		},
		{
			name:  "response resource",
			valid: true,
			data:  `{"apiVersion":"authproxy.net/v1alpha1","kind":"Actor","metadata":{"id":"act_test0000000000001","name":"billing","namespace":"root.acme","createdAt":"2026-01-02T03:04:05Z","updatedAt":"2026-01-02T03:04:05Z"},"spec":{"externalId":"user-123","permissions":[]},"status":{"signingKeyConfigured":true}}`,
		},
		{
			name:  "missing external id",
			valid: false,
			data:  `{"apiVersion":"authproxy.net/v1alpha1","kind":"Actor","metadata":{"namespace":"root"},"spec":{}}`,
		},
		{
			name:  "wrong kind",
			valid: false,
			data:  `{"apiVersion":"authproxy.net/v1alpha1","kind":"Connection","metadata":{"namespace":"root"},"spec":{"externalId":"user-123"}}`,
		},
		{
			name:  "unknown property",
			valid: false,
			data:  `{"apiVersion":"authproxy.net/v1alpha1","kind":"Actor","metadata":{"namespace":"root"},"spec":{"externalId":"user-123","unknown":true}}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var value any
			require.NoError(t, json.Unmarshal([]byte(test.data), &value))
			err := schema.Validate(value)
			if test.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestActorPatchSchema(t *testing.T) {
	compiler := jsonschemav5.NewCompiler()
	addSchema(t, compiler, "../../common/schema.json")
	addSchema(t, compiler, "../../auth/schema.json")
	addSchema(t, compiler, "../meta/schema.json")
	addSchema(t, compiler, "../namespace/schema.json")
	addSchema(t, compiler, "../key/schema.json")
	id := addSchema(t, compiler, "./schema.json")
	schema, err := compiler.Compile(id + "#/$defs/ActorPatch")
	require.NoError(t, err)

	var valid any
	require.NoError(t, json.Unmarshal([]byte(`{"apiVersion":"authproxy.net/v1alpha1","kind":"Actor","metadata":{},"spec":{"signingKey":null}}`), &valid))
	require.NoError(t, schema.Validate(valid))

	var invalid any
	require.NoError(t, json.Unmarshal([]byte(`{"apiVersion":"authproxy.net/v1alpha1","kind":"Actor","metadata":{},"spec":{"externalId":null}}`), &invalid))
	require.Error(t, schema.Validate(invalid))
}
