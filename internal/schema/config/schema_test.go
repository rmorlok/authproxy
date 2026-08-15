package config

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rmorlok/authproxy/internal/util"
	jsonschemav5 "github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/require"
)

type schemaId struct {
	Id string `json:"$id"`
}

func loadSchema(t *testing.T, c *jsonschemav5.Compiler, path string) string {
	schemaBytes, err := os.ReadFile(path)
	require.NoError(t, err)

	var schemaId schemaId
	err = json.Unmarshal(schemaBytes, &schemaId)
	require.NoError(t, err)

	err = c.AddResource(schemaId.Id, bytes.NewReader(schemaBytes))
	require.NoError(t, err)

	return schemaId.Id
}

func Test_SchemaAgainstRealData(t *testing.T) {
	c := jsonschemav5.NewCompiler()

	_ = loadSchema(t, c, "../resources/namespace/schema.json")
	_ = loadSchema(t, c, "../auth/schema.json")
	_ = loadSchema(t, c, "../common/schema.json")
	_ = loadSchema(t, c, "../resources/connectors/schema-oauth.json")
	_ = loadSchema(t, c, "../resources/connectors/schema.json")
	_ = loadSchema(t, c, "../resources/key/schema.json")
	schemaId := loadSchema(t, c, "./schema.json")

	require.Equal(t, SchemaIdConfig, schemaId, "schema ID should be the same as the one in the schema")

	schema, err := c.Compile(schemaId)
	if err != nil {
		t.Fatalf("failed to read schema: %v", err)
	}

	files, err := filepath.Glob("test_data/*.yaml")
	if err != nil {
		t.Fatalf("failed to list test files: %v", err)
	}

	if len(files) == 0 {
		t.Fatalf("no test files found")
	}

	for _, cfgPath := range files {
		name := strings.TrimSuffix(filepath.Base(cfgPath), ".yaml")
		if !strings.HasPrefix(name, "valid") && !strings.HasPrefix(name, "invalid") {
			t.Fatalf("invalid test file name: %s; must start with valid or invalid", name)
		}

		t.Run(name, func(t *testing.T) {
			b, err := ioutil.ReadFile(cfgPath)
			if err != nil {
				t.Fatalf("failed to read %s: %v", cfgPath, err)
			}

			data, err := util.YamlBytesToJSON(b)
			if err != nil {
				t.Fatalf("failed to convert YAML to JSON: %v", err)
			}

			var v interface{}
			if err := json.Unmarshal(data, &v); err != nil {
				t.Fatalf("failed to unmarshal JSON: %v", err)
			}

			err = schema.Validate(v)
			valid := err == nil
			shouldBeValid := strings.HasPrefix(name, "valid")
			if shouldBeValid && !valid {
				t.Fatalf("%s should be valid against schema, got error: %v", cfgPath, err)
			}

			if !shouldBeValid && valid {
				t.Fatalf("%s should not be valid against schema, got no error", cfgPath)
			}
		})
	}
}

func Test_SchemaAppMetricsShape(t *testing.T) {
	c := jsonschemav5.NewCompiler()

	_ = loadSchema(t, c, "../resources/namespace/schema.json")
	_ = loadSchema(t, c, "../auth/schema.json")
	_ = loadSchema(t, c, "../common/schema.json")
	_ = loadSchema(t, c, "../resources/connectors/schema-oauth.json")
	_ = loadSchema(t, c, "../resources/connectors/schema.json")
	_ = loadSchema(t, c, "../resources/key/schema.json")
	schemaId := loadSchema(t, c, "./schema.json")

	schema, err := c.Compile(schemaId)
	require.NoError(t, err)

	require.NoError(t, schema.Validate(map[string]any{
		"appMetrics": map[string]any{
			"resourceSnapshotInterval": "15m",
			"database": map[string]any{
				"provider": "sqlite",
				"path":     "./tmp/app_metrics.db",
			},
			"requestEvents": map[string]any{
				"fullRequestRecording": "always",
			},
		},
	}))

	require.Error(t, schema.Validate(map[string]any{
		"httpLogging": map[string]any{
			"fullRequestRecording": "always",
		},
	}))

	require.Error(t, schema.Validate(map[string]any{
		"appMetrics": map[string]any{
			"autoMigrate": true,
			"database": map[string]any{
				"provider": "sqlite",
				"path":     "./tmp/app_metrics.db",
			},
		},
	}))

	require.NoError(t, schema.Validate(map[string]any{
		"connections": map[string]any{
			"setupTtl": "1s",
		},
		"tasks": map[string]any{
			"defaultRetention": "24h",
		},
	}))
}

type test struct {
	Name  string
	Valid bool
	Data  string
}

type entities struct {
	Name   string
	Schema string
	Tests  []test
}

func compileTestSchema(t *testing.T, schemaJSON string) *jsonschemav5.Schema {
	c := jsonschemav5.NewCompiler()

	_ = loadSchema(t, c, "../resources/namespace/schema.json")
	_ = loadSchema(t, c, "../auth/schema.json")
	_ = loadSchema(t, c, "../common/schema.json")
	_ = loadSchema(t, c, "../resources/connectors/schema-oauth.json")
	_ = loadSchema(t, c, "../resources/connectors/schema.json")
	_ = loadSchema(t, c, "../resources/key/schema.json")

	sid := loadSchema(t, c, "./schema.json")
	require.Equal(t, SchemaIdConfig, sid)

	testSchemaId := "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/config/test.json"
	err := c.AddResource(testSchemaId, strings.NewReader(strings.TrimSpace(schemaJSON)))
	require.NoError(t, err)

	schema, err := c.Compile(testSchemaId)
	require.NoError(t, err)

	return schema
}

func TestSchemaDefinitions(t *testing.T) {
	entities := []entities{
		{
			Name: "KeyData",
			Schema: `
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/config/test.json",
  "type": "object",
  "additionalProperties": false,
  "required": ["test"],
  "properties": {
	"test": {
		"$ref": "../resources/key/schema.json#/$defs/KeyData"
    }
  }
}`,
			Tests: []test{
				{
					Name:  "raw value",
					Valid: true,
					Data:  `{"test": {"value": "my-key-data"}}`,
				},
				{
					Name:  "base64",
					Valid: true,
					Data:  `{"test": {"base64": "c29tZWtleQ=="}}`,
				},
				{
					Name:  "env var",
					Valid: true,
					Data:  `{"test": {"envVar": "MY_KEY"}}`,
				},
				{
					Name:  "env var base64",
					Valid: true,
					Data:  `{"test": {"envVarBase64": "MY_KEY_B64"}}`,
				},
				{
					Name:  "file path",
					Valid: true,
					Data:  `{"test": {"path": "/path/to/key"}}`,
				},
				{
					Name:  "num bytes",
					Valid: true,
					Data:  `{"test": {"numBytes": 32}}`,
				},
				{
					Name:  "aws kms",
					Valid: true,
					Data:  `{"test": {"awsKmsKeyId": "alias/authproxy", "awsRegion": "us-east-1", "awsKmsEndpoint": "http://localhost:4566", "awsCredentials": {"type": "implicit"}, "cacheTtl": "5m"}}`,
				},
				{
					Name:  "gcp kms full resource",
					Valid: true,
					Data:  `{"test": {"gcpKmsKeyName": "projects/test-project/locations/global/keyRings/authproxy/cryptoKeys/dek-wrapper", "gcpKmsEndpoint": "localhost:8085", "gcpCredentialsJson": {"envVar": "GCP_CREDS_JSON"}, "cacheTtl": "5m"}}`,
				},
				{
					Name:  "gcp kms components",
					Valid: true,
					Data:  `{"test": {"gcpProject": "test-project", "gcpLocation": "global", "gcpKeyRing": "authproxy", "gcpCryptoKey": "dek-wrapper", "gcpCredentialsFile": "/tmp/gcp-creds.json", "cacheTtl": "5m"}}`,
				},
				{
					Name:  "gcp kms missing component",
					Valid: false,
					Data:  `{"test": {"gcpProject": "test-project", "gcpLocation": "global", "gcpCryptoKey": "dek-wrapper"}}`,
				},
				{
					Name:  "vault transit",
					Valid: true,
					Data:  `{"test": {"vaultAddress": "http://127.0.0.1:8200", "vaultToken": "dev-only-token", "vaultNamespace": "admin", "vaultTransitMountPath": "transit", "vaultTransitKeyName": "authproxy", "cacheTtl": "5m"}}`,
				},
				{
					Name:  "vault transit missing key name",
					Valid: false,
					Data:  `{"test": {"vaultAddress": "http://127.0.0.1:8200", "vaultTransitMountPath": "transit"}}`,
				},
				{
					Name:  "empty object",
					Valid: false,
					Data:  `{"test": {}}`,
				},
				{
					Name:  "unknown property",
					Valid: false,
					Data:  `{"test": {"foo": "bar"}}`,
				},
				{
					Name:  "wrong type for value",
					Valid: false,
					Data:  `{"test": {"value": 123}}`,
				},
				{
					Name:  "num_bytes with string",
					Valid: false,
					Data:  `{"test": {"numBytes": "32"}}`,
				},
			},
		},
		{
			Name: "Key",
			Schema: `
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/config/test.json",
  "type": "object",
  "additionalProperties": false,
  "required": ["test"],
  "properties": {
	"test": {
		"$ref": "../resources/key/schema.json#/$defs/Key"
    }
  }
}`,
			Tests: []test{
				{
					Name:  "shared key",
					Valid: true,
					Data:  `{"test": {"sharedKey": {"value": "my-shared-key"}}}`,
				},
				{
					Name:  "public/private key",
					Valid: true,
					Data:  `{"test": {"publicKey": {"path": "/keys/pub"}, "privateKey": {"path": "/keys/priv"}}}`,
				},
				{
					Name:  "shared key with env var",
					Valid: true,
					Data:  `{"test": {"sharedKey": {"envVar": "MY_KEY"}}}`,
				},
			},
		},
		{
			Name: "TlsConfig",
			Schema: `
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/config/test.json",
  "type": "object",
  "additionalProperties": false,
  "required": ["test"],
  "properties": {
	"test": {
		"$ref": "./schema.json#/$defs/TlsConfig"
    }
  }
}`,
			Tests: []test{
				{
					Name:  "cert and key",
					Valid: true,
					Data:  `{"test": {"cert": {"path": "/certs/cert.pem"}, "key": {"path": "/certs/key.pem"}}}`,
				},
				{
					Name:  "lets encrypt",
					Valid: true,
					Data:  `{"test": {"acceptTos": true, "email": "admin@example.com", "cacheDir": "/certs"}}`,
				},
				{
					Name:  "lets encrypt with host whitelist",
					Valid: true,
					Data:  `{"test": {"acceptTos": true, "email": "admin@example.com", "cacheDir": "/certs", "hostWhitelist": ["example.com"]}}`,
				},
				{
					Name:  "lets encrypt with renew_before",
					Valid: true,
					Data:  `{"test": {"acceptTos": true, "email": "admin@example.com", "cacheDir": "/certs", "renewBefore": "30d"}}`,
				},
				{
					Name:  "self-signed autogen",
					Valid: true,
					Data:  `{"test": {"autoGenPath": "/certs/autogen"}}`,
				},
				{
					Name:  "cert without key",
					Valid: false,
					Data:  `{"test": {"cert": {"path": "/certs/cert.pem"}}}`,
				},
				{
					Name:  "lets encrypt without accept_tos",
					Valid: false,
					Data:  `{"test": {"email": "admin@example.com", "cacheDir": "/certs"}}`,
				},
			},
		},
		{
			Name: "Database",
			Schema: `
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/config/test.json",
  "type": "object",
  "additionalProperties": false,
  "required": ["test"],
  "properties": {
	"test": {
		"$ref": "./schema.json#/$defs/Database"
    }
  }
}`,
			Tests: []test{
				{
					Name:  "sqlite minimal",
					Valid: true,
					Data:  `{"test": {"provider": "sqlite", "path": "/data/db.sqlite"}}`,
				},
				{
					Name:  "sqlite rejects removed auto_migrate",
					Valid: false,
					Data:  `{"test": {"provider": "sqlite", "path": "/data/db.sqlite", "autoMigrate": true}}`,
				},
				{
					Name:  "sqlite with auto_migration_lock_duration",
					Valid: true,
					Data:  `{"test": {"provider": "sqlite", "path": "/data/db.sqlite", "autoMigrationLockDuration": "30s"}}`,
				},
				{
					Name:  "postgres minimal",
					Valid: true,
					Data:  `{"test": {"provider": "postgres", "host": "localhost"}}`,
				},
				{
					Name:  "postgres full",
					Valid: true,
					Data:  `{"test": {"provider": "postgres", "host": "example", "port": 1234, "user": "bobdole", "password": "secret", "sslmode": "disable"}}`,
				},
				{
					Name:  "postgres rejects removed auto_migrate",
					Valid: false,
					Data:  `{"test": {"provider": "postgres", "host": "localhost", "autoMigrate": true}}`,
				},
				{
					Name:  "postgres with auto_migration_lock_duration",
					Valid: true,
					Data:  `{"test": {"provider": "postgres", "host": "localhost", "autoMigrationLockDuration": "30s"}}`,
				},
				{
					Name:  "missing provider",
					Valid: false,
					Data:  `{"test": {"path": "/data/db.sqlite"}}`,
				},
				{
					Name:  "missing path",
					Valid: false,
					Data:  `{"test": {"provider": "sqlite"}}`,
				},
				{
					Name:  "wrong provider",
					Valid: false,
					Data:  `{"test": {"provider": "postgres", "path": "/data/db"}}`,
				},
				{
					Name:  "extra property",
					Valid: false,
					Data:  `{"test": {"provider": "sqlite", "path": "/data/db.sqlite", "extra": "field"}}`,
				},
			},
		},
		{
			Name: "DatabaseOrWarehouse",
			Schema: `
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/config/test.json",
  "type": "object",
  "additionalProperties": false,
  "required": ["test"],
  "properties": {
	"test": {
		"$ref": "./schema.json#/$defs/DatabaseOrWarehouse"
    }
  }
}`,
			Tests: []test{
				{
					Name:  "sqlite minimal",
					Valid: true,
					Data:  `{"test": {"provider": "sqlite", "path": "/data/db.sqlite"}}`,
				},
				{
					Name:  "sqlite warehouse rejects removed auto_migrate",
					Valid: false,
					Data:  `{"test": {"provider": "sqlite", "path": "/data/db.sqlite", "autoMigrate": true}}`,
				},
				{
					Name:  "sqlite with auto_migration_lock_duration",
					Valid: true,
					Data:  `{"test": {"provider": "sqlite", "path": "/data/db.sqlite", "autoMigrationLockDuration": "30s"}}`,
				},
				{
					Name:  "postgres minimal",
					Valid: true,
					Data:  `{"test": {"provider": "postgres", "host": "localhost"}}`,
				},
				{
					Name:  "postgres full",
					Valid: true,
					Data:  `{"test": {"provider": "postgres", "host": "example", "port": 1234, "user": "bobdole", "password": "secret", "sslmode": "disable"}}`,
				},
				{
					Name:  "postgres warehouse rejects removed auto_migrate",
					Valid: false,
					Data:  `{"test": {"provider": "postgres", "host": "localhost", "autoMigrate": true}}`,
				},
				{
					Name:  "postgres with auto_migration_lock_duration",
					Valid: true,
					Data:  `{"test": {"provider": "postgres", "host": "localhost", "autoMigrationLockDuration": "30s"}}`,
				},
				{
					Name:  "clickhouse minimal",
					Valid: true,
					Data:  `{"test": {"provider": "clickhouse", "address": "localhost"}}`,
				},
				{
					Name:  "clickhouse full - addresses",
					Valid: true,
					Data:  `{"test": {"provider": "clickhouse", "addresses": ["host1", "host2"], "user": "bobdole", "password": "secret", "database": "authproxy"}}`,
				},
				{
					Name:  "clickhouse full - address",
					Valid: true,
					Data:  `{"test": {"provider": "clickhouse", "address": "host1", "user": "bobdole", "password": "secret", "database": "authproxy"}}`,
				},
				{
					Name:  "clickhouse full - address list",
					Valid: true,
					Data:  `{"test": {"provider": "clickhouse", "addressList": "host1,host2", "user": "bobdole", "password": "secret", "database": "authproxy"}}`,
				},
				{
					Name:  "clickhouse rejects removed auto_migrate",
					Valid: false,
					Data:  `{"test": {"provider": "clickhouse", "address": "localhost", "autoMigrate": true}}`,
				},
				{
					Name:  "clickhouse with auto_migration_lock_duration",
					Valid: true,
					Data:  `{"test": {"provider": "clickhouse", "address": "localhost", "autoMigrationLockDuration": "30s"}}`,
				},
				{
					Name:  "clickhouse with protocol http",
					Valid: true,
					Data:  `{"test": {"provider": "clickhouse", "address": "localhost", "protocol": "http"}}`,
				},
				{
					Name:  "clickhouse with protocol native",
					Valid: true,
					Data:  `{"test": {"provider": "clickhouse", "address": "localhost", "protocol": "native"}}`,
				},
				{
					Name:  "clickhouse with invalid protocol",
					Valid: false,
					Data:  `{"test": {"provider": "clickhouse", "address": "localhost", "protocol": "grpc"}}`,
				},
				{
					Name:  "missing provider",
					Valid: false,
					Data:  `{"test": {"path": "/data/db.sqlite"}}`,
				},
				{
					Name:  "missing path",
					Valid: false,
					Data:  `{"test": {"provider": "sqlite"}}`,
				},
				{
					Name:  "wrong provider",
					Valid: false,
					Data:  `{"test": {"provider": "postgres", "path": "/data/db"}}`,
				},
				{
					Name:  "extra property",
					Valid: false,
					Data:  `{"test": {"provider": "sqlite", "path": "/data/db.sqlite", "extra": "field"}}`,
				},
			},
		},
		{
			Name: "LoggingConfig",
			Schema: `
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/config/test.json",
  "type": "object",
  "additionalProperties": false,
  "required": ["test"],
  "properties": {
	"test": {
		"$ref": "./schema.json#/$defs/LoggingConfig"
    }
  }
}`,
			Tests: []test{
				{
					Name:  "none",
					Valid: true,
					Data:  `{"test": {"type": "none"}}`,
				},
				{
					Name:  "text minimal",
					Valid: true,
					Data:  `{"test": {"type": "text"}}`,
				},
				{
					Name:  "text with options",
					Valid: true,
					Data:  `{"test": {"type": "text", "to": "stderr", "level": "debug", "source": true}}`,
				},
				{
					Name:  "json minimal",
					Valid: true,
					Data:  `{"test": {"type": "json"}}`,
				},
				{
					Name:  "json with options",
					Valid: true,
					Data:  `{"test": {"type": "json", "to": "stdout", "level": "info"}}`,
				},
				{
					Name:  "tint minimal",
					Valid: true,
					Data:  `{"test": {"type": "tint"}}`,
				},
				{
					Name:  "tint with all options",
					Valid: true,
					Data:  `{"test": {"type": "tint", "to": "stderr", "level": "warn", "source": false, "noColor": true, "timeFormat": "15:04:05"}}`,
				},
				{
					Name:  "missing type",
					Valid: false,
					Data:  `{"test": {}}`,
				},
				{
					Name:  "unknown type",
					Valid: false,
					Data:  `{"test": {"type": "unknown"}}`,
				},
				{
					Name:  "text with extra property",
					Valid: false,
					Data:  `{"test": {"type": "text", "noColor": true}}`,
				},
				{
					Name:  "none with extra property",
					Valid: false,
					Data:  `{"test": {"type": "none", "level": "debug"}}`,
				},
			},
		},
		{
			Name: "Redis",
			Schema: `
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/config/test.json",
  "type": "object",
  "additionalProperties": false,
  "required": ["test"],
  "properties": {
	"test": {
		"$ref": "./schema.json#/$defs/Redis"
    }
  }
}`,
			Tests: []test{
				{
					Name:  "redis minimal",
					Valid: true,
					Data:  `{"test": {"address": "localhost:6379"}}`,
				},
				{
					Name:  "redis with provider",
					Valid: true,
					Data:  `{"test": {"provider": "redis", "address": "localhost:6379"}}`,
				},
				{
					Name:  "redis with all options",
					Valid: true,
					Data:  `{"test": {"provider": "redis", "address": "localhost:6379", "network": "tcp", "protocol": 2, "username": "user", "password": {"envVar": "REDIS_PASS"}, "db": 1}}`,
				},
				{
					Name:  "miniredis",
					Valid: true,
					Data:  `{"test": {"provider": "miniredis"}}`,
				},
				{
					Name:  "redis missing address",
					Valid: false,
					Data:  `{"test": {"provider": "redis"}}`,
				},
				{
					Name:  "redis extra property",
					Valid: false,
					Data:  `{"test": {"provider": "redis", "address": "localhost:6379", "extra": "field"}}`,
				},
				{
					Name:  "miniredis extra property",
					Valid: false,
					Data:  `{"test": {"provider": "miniredis", "extra": "field"}}`,
				},
			},
		},
		{
			Name: "BlobStorage",
			Schema: `
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/config/test.json",
  "type": "object",
  "additionalProperties": false,
  "required": ["test"],
  "properties": {
	"test": {
		"$ref": "./schema.json#/$defs/BlobStorage"
    }
  }
}`,
			Tests: []test{
				{
					Name:  "s3 minimal",
					Valid: true,
					Data:  `{"test": {"provider": "s3", "bucket": "request-logs"}}`,
				},
				{
					Name:  "memory",
					Valid: true,
					Data:  `{"test": {"provider": "memory"}}`,
				},
				{
					Name:  "filesystem",
					Valid: true,
					Data:  `{"test": {"provider": "filesystem", "path": "/tmp/authproxy/blobs"}}`,
				},
				{
					Name:  "filesystem missing path",
					Valid: false,
					Data:  `{"test": {"provider": "filesystem"}}`,
				},
				{
					Name:  "filesystem extra property",
					Valid: false,
					Data:  `{"test": {"provider": "filesystem", "path": "/tmp/authproxy/blobs", "bucket": "x"}}`,
				},
				{
					Name:  "unknown provider",
					Valid: false,
					Data:  `{"test": {"provider": "unknown"}}`,
				},
			},
		},
		{
			Name: "ConfiguredActor",
			Schema: `
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/config/test.json",
  "type": "object",
  "additionalProperties": false,
  "required": ["test"],
  "properties": {
	"test": {
		"$ref": "./schema.json#/$defs/ConfiguredActor"
    }
  }
}`,
			Tests: []test{
				{
					Name:  "minimal",
					Valid: true,
					Data:  `{"test": {"externalId": "actor-1", "key": {"sharedKey": {"value": "secret"}}}}`,
				},
				{
					Name:  "with permissions",
					Valid: true,
					Data:  `{"test": {"externalId": "actor-1", "key": {"sharedKey": {"value": "secret"}}, "permissions": [{"namespace": "root", "resources": ["*"], "verbs": ["*"]}]}}`,
				},
				{
					Name:  "with labels",
					Valid: true,
					Data:  `{"test": {"externalId": "actor-1", "key": {"sharedKey": {"value": "secret"}}, "labels": {"env": "prod"}}}`,
				},
				{
					Name:  "missing external_id",
					Valid: false,
					Data:  `{"test": {"key": {"sharedKey": {"value": "secret"}}}}`,
				},
				{
					Name:  "missing key",
					Valid: false,
					Data:  `{"test": {"externalId": "actor-1"}}`,
				},
			},
		},
		{
			Name: "ConfiguredActors",
			Schema: `
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/config/test.json",
  "type": "object",
  "additionalProperties": false,
  "required": ["test"],
  "properties": {
	"test": {
		"$ref": "./schema.json#/$defs/ConfiguredActors"
    }
  }
}`,
			Tests: []test{
				{
					Name:  "inline list",
					Valid: true,
					Data:  `{"test": [{"externalId": "actor-1", "key": {"sharedKey": {"value": "secret"}}}]}`,
				},
				{
					Name:  "external source",
					Valid: true,
					Data:  `{"test": {"keysPath": "/keys/actors"}}`,
				},
				{
					Name:  "external source with permissions",
					Valid: true,
					Data:  `{"test": {"keysPath": "/keys/actors", "permissions": [{"namespace": "root.**", "resources": ["*"], "verbs": ["*"]}]}}`,
				},
				{
					Name:  "external source with sync cron",
					Valid: true,
					Data:  `{"test": {"keysPath": "/keys/actors", "syncCronSchedule": "*/5 * * * *"}}`,
				},
				{
					Name:  "external source missing keys_path",
					Valid: false,
					Data:  `{"test": {"permissions": [{"namespace": "root", "resources": ["*"], "verbs": ["*"]}]}}`,
				},
			},
		},
		{
			Name: "AdminUser",
			Schema: `
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/config/test.json",
  "type": "object",
  "additionalProperties": false,
  "required": ["test"],
  "properties": {
	"test": {
		"$ref": "./schema.json#/$defs/AdminUser"
    }
  }
}`,
			Tests: []test{
				{
					Name:  "minimal",
					Valid: true,
					Data:  `{"test": {"username": "admin", "key": {"sharedKey": {"value": "secret"}}}}`,
				},
				{
					Name:  "with email",
					Valid: true,
					Data:  `{"test": {"username": "admin", "email": "admin@example.com", "key": {"sharedKey": {"value": "secret"}}}}`,
				},
				{
					Name:  "with permissions",
					Valid: true,
					Data:  `{"test": {"username": "admin", "key": {"sharedKey": {"value": "secret"}}, "permissions": [{"namespace": "root.**", "resources": ["*"], "verbs": ["*"]}]}}`,
				},
				{
					Name:  "missing username",
					Valid: false,
					Data:  `{"test": {"key": {"sharedKey": {"value": "secret"}}}}`,
				},
				{
					Name:  "missing key",
					Valid: false,
					Data:  `{"test": {"username": "admin"}}`,
				},
				{
					Name:  "extra property",
					Valid: false,
					Data:  `{"test": {"username": "admin", "key": {"sharedKey": {"value": "secret"}}, "extra": "field"}}`,
				},
			},
		},
		{
			Name: "AdminUsers",
			Schema: `
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/config/test.json",
  "type": "object",
  "additionalProperties": false,
  "required": ["test"],
  "properties": {
	"test": {
		"$ref": "./schema.json#/$defs/AdminUsers"
    }
  }
}`,
			Tests: []test{
				{
					Name:  "inline list",
					Valid: true,
					Data:  `{"test": [{"username": "admin", "key": {"sharedKey": {"value": "secret"}}}]}`,
				},
				{
					Name:  "external source",
					Valid: true,
					Data:  `{"test": {"keysPath": "/keys/admins"}}`,
				},
				{
					Name:  "external source with permissions",
					Valid: true,
					Data:  `{"test": {"keysPath": "/keys/admins", "permissions": [{"namespace": "root.**", "resources": ["*"], "verbs": ["*"]}]}}`,
				},
				{
					Name:  "external source with sync cron",
					Valid: true,
					Data:  `{"test": {"keysPath": "/keys/admins", "syncCronSchedule": "*/5 * * * *"}}`,
				},
				{
					Name:  "external source missing keys_path",
					Valid: false,
					Data:  `{"test": {"permissions": [{"namespace": "root", "resources": ["*"], "verbs": ["*"]}]}}`,
				},
			},
		},
		{
			Name: "SystemAuth",
			Schema: `
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/config/test.json",
  "type": "object",
  "additionalProperties": false,
  "required": ["test"],
  "properties": {
	"test": {
		"$ref": "./schema.json#/$defs/SystemAuth"
    }
  }
}`,
			Tests: []test{
				{
					Name:  "minimal",
					Valid: true,
					Data:  `{"test": {}}`,
				},
				{
					Name:  "with jwt signing key",
					Valid: true,
					Data:  `{"test": {"jwtSigningKey": {"sharedKey": {"value": "secret"}}, "jwtIssuer": "my-app"}}`,
				},
				{
					Name:  "jwt_token_duration as integer",
					Valid: true,
					Data:  `{"test": {"jwtTokenDuration": 3600000000000}}`,
				},
				{
					Name:  "jwt_token_duration as string is invalid",
					Valid: false,
					Data:  `{"test": {"jwtTokenDuration": "1h"}}`,
				},
				{
					Name:  "global_aes_key is KeyData not Key",
					Valid: true,
					Data:  `{"test": {"globalAesKey": {"envVarBase64": "GLOBAL_AES_KEY"}}}`,
				},
				{
					Name:  "data encryption key policy",
					Valid: true,
					Data:  `{"test": {"dataEncryptionKeys": {"rotationInterval": "24h", "ensureCurrent": true}}}`,
				},
				{
					Name:  "actors as external source",
					Valid: true,
					Data:  `{"test": {"actors": {"keysPath": "/keys/actors"}}}`,
				},
				{
					Name:  "actors as inline list",
					Valid: true,
					Data:  `{"test": {"actors": [{"externalId": "svc", "key": {"sharedKey": {"value": "secret"}}}]}}`,
				},
				{
					Name:  "extra property",
					Valid: false,
					Data:  `{"test": {"extra": "field"}}`,
				},
			},
		},
		{
			Name: "Connectors",
			Schema: `
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/config/test.json",
  "type": "object",
  "additionalProperties": false,
  "required": ["test"],
  "properties": {
	"test": {
		"$ref": "./schema.json#/$defs/Connectors"
    }
  }
}`,
			Tests: []test{
				{
					Name:  "minimal",
					Valid: true,
					Data:  `{"test": {}}`,
				},
				{
					Name:  "identifying_labels was removed",
					Valid: false,
					Data:  `{"test": {"identifyingLabels": ["type", "region"]}}`,
				},
				{
					Name:  "rejects removed auto_migrate",
					Valid: false,
					Data:  `{"test": {"autoMigrate": true, "autoMigrationLockDuration": "30s"}}`,
				},
				{
					Name:  "connector with name",
					Valid: true,
					Data:  `{"test":{"loadFromList":[{"name":"example","labels":{},"displayName":"Example","logo":{"publicUrl":"https://example.com/logo.svg"},"auth":{"type":"no-auth"}}]}}`,
				},
				{
					Name:  "connector without id or name",
					Valid: false,
					Data:  `{"test":{"loadFromList":[{"labels":{},"displayName":"Example","logo":{"publicUrl":"https://example.com/logo.svg"},"auth":{"type":"no-auth"}}]}}`,
				},
				{
					Name:  "extra property",
					Valid: false,
					Data:  `{"test": {"extra": "field"}}`,
				},
			},
		},
		{
			Name: "ErrorPages",
			Schema: `
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/config/test.json",
  "type": "object",
  "additionalProperties": false,
  "required": ["test"],
  "properties": {
	"test": {
		"$ref": "./schema.json#/$defs/ErrorPages"
    }
  }
}`,
			Tests: []test{
				{
					Name:  "minimal",
					Valid: true,
					Data:  `{"test": {}}`,
				},
				{
					Name:  "with urls",
					Valid: true,
					Data:  `{"test": {"notFound": "https://example.com/404", "unauthorized": "https://example.com/401"}}`,
				},
				{
					Name:  "template as string (StringValue)",
					Valid: true,
					Data:  `{"test": {"template": "<html>error</html>"}}`,
				},
				{
					Name:  "template as file path (StringValue)",
					Valid: true,
					Data:  `{"test": {"template": {"path": "/templates/error.html"}}}`,
				},
				{
					Name:  "template as env var (StringValue)",
					Valid: true,
					Data:  `{"test": {"template": {"envVar": "ERROR_TEMPLATE"}}}`,
				},
				{
					Name:  "extra property",
					Valid: false,
					Data:  `{"test": {"extra": "field"}}`,
				},
			},
		},
		{
			Name: "Marketplace",
			Schema: `
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/config/test.json",
  "type": "object",
  "additionalProperties": false,
  "required": ["test"],
  "properties": {
	"test": {
		"$ref": "./schema.json#/$defs/Marketplace"
    }
  }
}`,
			Tests: []test{
				{
					Name:  "base_url as string (StringValue)",
					Valid: true,
					Data:  `{"test": {"baseUrl": "http://localhost:5173"}}`,
				},
				{
					Name:  "base_url as env var (StringValue)",
					Valid: true,
					Data:  `{"test": {"baseUrl": {"envVar": "MARKETPLACE_URL"}}}`,
				},
				{
					Name:  "extra property",
					Valid: false,
					Data:  `{"test": {"extra": "field"}}`,
				},
			},
		},
		{
			Name: "ServiceAdminUi",
			Schema: `
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/config/test.json",
  "type": "object",
  "additionalProperties": false,
  "required": ["test"],
  "properties": {
	"test": {
		"$ref": "./schema.json#/$defs/ServiceAdminUi"
    }
  }
}`,
			Tests: []test{
				{
					Name:  "base_url as string (StringValue)",
					Valid: true,
					Data:  `{"test": {"enabled": true, "baseUrl": "http://localhost:5174"}}`,
				},
				{
					Name:  "base_url as env var (StringValue)",
					Valid: true,
					Data:  `{"test": {"enabled": true, "baseUrl": {"envVar": "ADMIN_UI_URL"}}}`,
				},
				{
					Name:  "extra property",
					Valid: false,
					Data:  `{"test": {"extra": "field"}}`,
				},
			},
		},
		{
			Name: "ServiceApi port as StringValue",
			Schema: `
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/config/test.json",
  "type": "object",
  "additionalProperties": false,
  "required": ["test"],
  "properties": {
	"test": {
		"$ref": "./schema.json#/$defs/ServiceApi"
    }
  }
}`,
			Tests: []test{
				{
					Name:  "port as number",
					Valid: true,
					Data:  `{"test": {"port": 8080}}`,
				},
				{
					Name:  "port as env var",
					Valid: true,
					Data:  `{"test": {"port": {"envVar": "API_PORT"}}}`,
				},
				{
					Name:  "health_check_port as number",
					Valid: true,
					Data:  `{"test": {"port": 8080, "healthCheckPort": 8081}}`,
				},
				{
					Name:  "base_url as string",
					Valid: true,
					Data:  `{"test": {"port": 8080, "baseUrl": "https://api.example.com"}}`,
				},
				{
					Name:  "port as string is invalid for IntegerValue",
					Valid: false,
					Data:  `{"test": {"port": "bad"}}`,
				},
				{
					Name:  "extra property",
					Valid: false,
					Data:  `{"test": {"extra": "field"}}`,
				},
			},
		},
		{
			Name: "ServiceWorker workflow options",
			Schema: `
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/config/test.json",
  "type": "object",
  "additionalProperties": false,
  "required": ["test"],
  "properties": {
	"test": {
		"$ref": "./schema.json#/$defs/ServiceWorker"
    }
  }
}`,
			Tests: []test{
				{
					Name:  "workflow options as strings",
					Valid: true,
					Data:  `{"test": {"workflowPollers": "3", "activityPollers": "4", "maxParallelWorkflowTasks": "5", "maxParallelActivityTasks": "6", "workflowHeartbeatInterval": "7s"}}`,
				},
				{
					Name:  "workflow options as integer equivalents",
					Valid: true,
					Data:  `{"test": {"workflowPollers": 3, "activityPollers": 4, "maxParallelWorkflowTasks": 5, "maxParallelActivityTasks": 6}}`,
				},
				{
					Name:  "extra property",
					Valid: false,
					Data:  `{"test": {"workflowPollers": "3", "extra": "field"}}`,
				},
			},
		},
		{
			Name: "HostApplication",
			Schema: `
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/config/test.json",
  "type": "object",
  "additionalProperties": false,
  "required": ["test"],
  "properties": {
	"test": {
		"$ref": "./schema.json#/$defs/HostApplication"
    }
  }
}`,
			Tests: []test{
				{
					Name:  "with initiate_session_url",
					Valid: true,
					Data:  `{"test": {"initiateSessionUrl": "http://localhost:8888/login"}}`,
				},
				{
					Name:  "empty is valid",
					Valid: true,
					Data:  `{"test": {}}`,
				},
				{
					Name:  "extra property not allowed",
					Valid: false,
					Data:  `{"test": {"initiateSessionUrl": "http://localhost:8888/login", "extra": "field"}}`,
				},
			},
		},
	}

	for _, entity := range entities {
		t.Run(entity.Name, func(t *testing.T) {
			schema := compileTestSchema(t, entity.Schema)

			for _, test := range entity.Tests {
				t.Run(test.Name, func(t *testing.T) {
					var v interface{}
					if err := json.Unmarshal([]byte(test.Data), &v); err != nil {
						t.Fatalf("failed to unmarshal JSON: %v", err)
					}

					err := schema.Validate(v)
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
