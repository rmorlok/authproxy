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
				{"env var", true, `{"test": {"env_var": "MY_KEY"}}`},
				{"env var base64", true, `{"test": {"env_var_base64": "MY_KEY_B64"}}`},
				{"file path", true, `{"test": {"path": "/path/to/key"}}`},
				{"num bytes", true, `{"test": {"num_bytes": 32}}`},
				{"random sentinel", true, `{"test": {"random": true}}`},
				{"aws kms", true, `{"test": {"aws_kms_key_id": "alias/authproxy", "aws_region": "us-east-1", "aws_kms_endpoint": "http://localhost:4566", "aws_credentials": {"type": "implicit"}, "cache_ttl": "5m"}}`},
				{"aws secret", true, `{"test": {"aws_secret_id": "authproxy/global", "aws_secret_key": "data", "aws_region": "us-east-1", "aws_credentials": {"type": "implicit"}, "cache_ttl": "5m"}}`},
				{"gcp kms full resource", true, `{"test": {"gcp_kms_key_name": "projects/test-project/locations/global/keyRings/authproxy/cryptoKeys/dek-wrapper", "gcp_kms_endpoint": "localhost:8085", "gcp_credentials_json": {"env_var": "GCP_CREDS_JSON"}, "cache_ttl": "5m"}}`},
				{"gcp kms components", true, `{"test": {"gcp_project": "test-project", "gcp_location": "global", "gcp_key_ring": "authproxy", "gcp_crypto_key": "dek-wrapper", "gcp_credentials_file": "/tmp/gcp-creds.json", "cache_ttl": "5m"}}`},
				{"gcp kms missing component", false, `{"test": {"gcp_project": "test-project", "gcp_location": "global", "gcp_crypto_key": "dek-wrapper"}}`},
				{"gcp secret", true, `{"test": {"gcp_secret_name": "authproxy-key", "gcp_project": "test-project", "gcp_secret_version": "latest", "cache_ttl": "5m"}}`},
				{"vault kv", true, `{"test": {"vault_address": "http://127.0.0.1:8200", "vault_token": "dev-only-token", "vault_path": "secret/data/authproxy", "vault_key": "value", "cache_ttl": "5m"}}`},
				{"vault transit", true, `{"test": {"vault_address": "http://127.0.0.1:8200", "vault_token": "dev-only-token", "vault_namespace": "admin", "vault_transit_mount_path": "transit", "vault_transit_key_name": "authproxy", "cache_ttl": "5m"}}`},
				{"vault transit missing key name", false, `{"test": {"vault_address": "http://127.0.0.1:8200", "vault_transit_mount_path": "transit"}}`},
				{"mock", true, `{"test": {"mock_id": "unit-test"}}`},
				{"mock kms", true, `{"test": {"mock_kms_id": "unit-test"}}`},
				{"empty object", false, `{"test": {}}`},
				{"unknown property", false, `{"test": {"foo": "bar"}}`},
				{"wrong type for value", false, `{"test": {"value": 123}}`},
				{"num_bytes with string", false, `{"test": {"num_bytes": "32"}}`},
			},
		},
		{
			Name:   "Key",
			Schema: mkSchema("./schema.json#/$defs/Key"),
			Tests: []testCase{
				{"shared key", true, `{"test": {"shared_key": {"value": "my-shared-key"}}}`},
				{"public key", true, `{"test": {"public_key": {"path": "/keys/pub"}}}`},
				{"private key", true, `{"test": {"private_key": {"path": "/keys/priv"}}}`},
				{"public/private key", true, `{"test": {"public_key": {"path": "/keys/pub"}, "private_key": {"path": "/keys/priv"}}}`},
				{"shared key with env var", true, `{"test": {"shared_key": {"env_var": "MY_KEY"}}}`},
				{"empty object", false, `{"test": {}}`},
				{"unknown property", false, `{"test": {"foo": {"value": "x"}}}`},
			},
		},
	}

	for _, entity := range entities {
		t.Run(entity.Name, func(t *testing.T) {
			for _, test := range entity.Tests {
				t.Run(test.Name, func(t *testing.T) {
					c := jsonschemav5.NewCompiler()
					_ = loadSchemaInto(t, c, "../../common/schema.json")
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
