package key

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	jsonschemav5 "github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/require"
)

type schemaIdEnvelope struct {
	Id string `json:"$id"`
}

func loadSchemaInto(t *testing.T, c *jsonschemav5.Compiler, path string) string {
	schemaBytes, err := os.ReadFile(path)
	require.NoError(t, err)

	var sid schemaIdEnvelope
	require.NoError(t, json.Unmarshal(schemaBytes, &sid))
	require.NoError(t, c.AddResource(sid.Id, bytes.NewReader(schemaBytes)))
	return sid.Id
}

func TestSchemaId(t *testing.T) {
	c := jsonschemav5.NewCompiler()
	_ = loadSchemaInto(t, c, "../../common/schema.json")
	_ = loadSchemaInto(t, c, "../meta/schema.json")
	_ = loadSchemaInto(t, c, "../namespace/schema.json")
	id := loadSchemaInto(t, c, "./schema.json")
	require.Equal(t, SchemaIdKey, id)
}

func TestSchema(t *testing.T) {
	type testCase struct {
		Name  string
		Valid bool
		Data  string
	}

	type entity struct {
		Name   string
		Schema string
		Tests  []testCase
	}

	const testSchemaId = "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/resources/key/test.json"
	mkSchema := func(ref string) string {
		return strings.TrimSpace(`
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "` + testSchemaId + `",
  "type": "object",
  "additionalProperties": false,
  "required": ["test"],
  "properties": {
    "test": { "$ref": "` + ref + `" }
  }
}`)
	}

	entities := []entity{
		{
			Name:   "KeyData",
			Schema: mkSchema("./schema.json#/$defs/KeyData"),
			Tests: []testCase{
				{"raw value", true, `{"test": {"value": "my-key-data"}}`},
				{"base64", true, `{"test": {"base64": "c29tZWtleQ=="}}`},
				{"env var", true, `{"test": {"envVar": "MY_KEY"}}`},
				{"env var base64", true, `{"test": {"envVarBase64": "MY_KEY_B64"}}`},
				{"file path", true, `{"test": {"path": "/path/to/key"}}`},
				{"num bytes", true, `{"test": {"numBytes": 32}}`},
				{"random sentinel", true, `{"test": {"random": true}}`},
				{"aws kms", true, `{"test": {"awsKmsKeyId": "alias/authproxy", "awsRegion": "us-east-1", "awsKmsEndpoint": "http://localhost:4566", "awsCredentials": {"type": "implicit"}, "cacheTtl": "5m"}}`},
				{"aws secret", true, `{"test": {"awsSecretId": "authproxy/global", "awsSecretKey": "data", "awsRegion": "us-east-1", "awsCredentials": {"type": "implicit"}, "cacheTtl": "5m"}}`},
				{"gcp kms full resource", true, `{"test": {"gcpKmsKeyName": "projects/test-project/locations/global/keyRings/authproxy/cryptoKeys/dek-wrapper", "gcpKmsEndpoint": "localhost:8085", "gcpCredentialsJson": {"envVar": "GCP_CREDS_JSON"}, "cacheTtl": "5m"}}`},
				{"gcp kms components", true, `{"test": {"gcpProject": "test-project", "gcpLocation": "global", "gcpKeyRing": "authproxy", "gcpCryptoKey": "dek-wrapper", "gcpCredentialsFile": "/tmp/gcp-creds.json", "cacheTtl": "5m"}}`},
				{"gcp kms missing component", false, `{"test": {"gcpProject": "test-project", "gcpLocation": "global", "gcpCryptoKey": "dek-wrapper"}}`},
				{"gcp secret", true, `{"test": {"gcpSecretName": "authproxy-key", "gcpProject": "test-project", "gcpSecretVersion": "latest", "cacheTtl": "5m"}}`},
				{"vault kv", true, `{"test": {"vaultAddress": "http://127.0.0.1:8200", "vaultToken": "dev-only-token", "vaultPath": "secret/data/authproxy", "vaultKey": "value", "cacheTtl": "5m"}}`},
				{"vault transit", true, `{"test": {"vaultAddress": "http://127.0.0.1:8200", "vaultToken": "dev-only-token", "vaultNamespace": "admin", "vaultTransitMountPath": "transit", "vaultTransitKeyName": "authproxy", "cacheTtl": "5m"}}`},
				{"vault transit missing key name", false, `{"test": {"vaultAddress": "http://127.0.0.1:8200", "vaultTransitMountPath": "transit"}}`},
				{"mock", true, `{"test": {"mockId": "unit-test"}}`},
				{"mock kms", true, `{"test": {"mockKmsId": "unit-test"}}`},
				{"empty object", false, `{"test": {}}`},
				{"unknown property", false, `{"test": {"foo": "bar"}}`},
				{"wrong type for value", false, `{"test": {"value": 123}}`},
				{"num_bytes with string", false, `{"test": {"numBytes": "32"}}`},
			},
		},
		{
			Name:   "SigningKey",
			Schema: mkSchema("./schema.json#/$defs/SigningKey"),
			Tests: []testCase{
				{"shared key", true, `{"test": {"sharedKey": {"value": "my-shared-key"}}}`},
				{"public key", true, `{"test": {"publicKey": {"path": "/keys/pub"}}}`},
				{"private key", true, `{"test": {"privateKey": {"path": "/keys/priv"}}}`},
				{"public/private key", true, `{"test": {"publicKey": {"path": "/keys/pub"}, "privateKey": {"path": "/keys/priv"}}}`},
				{"shared key with env var", true, `{"test": {"sharedKey": {"envVar": "MY_KEY"}}}`},
				{"empty object", false, `{"test": {}}`},
				{"unknown property", false, `{"test": {"foo": {"value": "x"}}}`},
			},
		},
		{
			Name:   "ManagedKey",
			Schema: mkSchema("./schema.json"),
			Tests: []testCase{
				{"resource", true, `{"test":{"apiVersion":"authproxy.net/v1alpha1","kind":"Key","metadata":{"id":"key_test550e8400abcd","name":"primary-key","namespace":"root.acme"},"spec":{"usage":"data_encryption","materialType":"symmetric","desiredState":"active","keyData":{"value":"***"}},"status":{"state":"active","keyDataConfigured":true}}}`},
				{"create defaults omitted", true, `{"test":{"apiVersion":"authproxy.net/v1alpha1","kind":"Key","metadata":{"namespace":"root"},"spec":{"keyData":{"numBytes":32}}}}`},
				{"wrong kind", false, `{"test":{"apiVersion":"authproxy.net/v1alpha1","kind":"Connector","metadata":{"namespace":"root"},"spec":{"keyData":{"numBytes":32}}}}`},
				{"legacy flat shape", false, `{"test":{"id":"key_test550e8400abcd","namespace":"root","state":"active"}}`},
			},
		},
		{
			Name:   "ManagedKeyPatch",
			Schema: mkSchema("./schema.json#/$defs/KeyPatch"),
			Tests: []testCase{
				{"desired state", true, `{"test":{"apiVersion":"authproxy.net/v1alpha1","kind":"Key","metadata":{},"spec":{"desiredState":"disabled"}}}`},
				{"provider update", true, `{"test":{"apiVersion":"authproxy.net/v1alpha1","kind":"Key","metadata":{},"spec":{"keyData":{"value":"replacement"}}}}`},
				{"null provider", false, `{"test":{"apiVersion":"authproxy.net/v1alpha1","kind":"Key","metadata":{},"spec":{"keyData":null}}}`},
				{"legacy flat patch", false, `{"test":{"state":"disabled"}}`},
			},
		},
	}

	for _, entity := range entities {
		t.Run(entity.Name, func(t *testing.T) {
			for _, test := range entity.Tests {
				t.Run(test.Name, func(t *testing.T) {
					c := jsonschemav5.NewCompiler()
					_ = loadSchemaInto(t, c, "../../common/schema.json")
					_ = loadSchemaInto(t, c, "../meta/schema.json")
					_ = loadSchemaInto(t, c, "../namespace/schema.json")
					schemaID := loadSchemaInto(t, c, "./schema.json")
					require.Equal(t, SchemaIdKey, schemaID)
					require.NoError(t, c.AddResource(testSchemaId, strings.NewReader(entity.Schema)))

					schema, err := c.Compile(testSchemaId)
					require.NoError(t, err)

					var v interface{}
					require.NoError(t, json.Unmarshal([]byte(test.Data), &v))

					err = schema.Validate(v)
					if test.Valid {
						require.NoError(t, err)
					} else {
						require.Error(t, err)
					}
				})
			}
		})
	}
}
