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

	_ = loadSchema(t, c, "../auth/schema.json")
	_ = loadSchema(t, c, "../common/schema.json")
	_ = loadSchema(t, c, "../connectors/schema-oauth.json")
	_ = loadSchema(t, c, "../connectors/schema.json")
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

	_ = loadSchema(t, c, "../auth/schema.json")
	_ = loadSchema(t, c, "../common/schema.json")
	_ = loadSchema(t, c, "../connectors/schema-oauth.json")
	_ = loadSchema(t, c, "../connectors/schema.json")

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
		"$ref": "./schema.json#/$defs/KeyData"
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
					Data:  `{"test": {"env_var": "MY_KEY"}}`,
				},
				{
					Name:  "env var base64",
					Valid: true,
					Data:  `{"test": {"env_var_base64": "MY_KEY_B64"}}`,
				},
				{
					Name:  "file path",
					Valid: true,
					Data:  `{"test": {"path": "/path/to/key"}}`,
				},
				{
					Name:  "num bytes",
					Valid: true,
					Data:  `{"test": {"num_bytes": 32}}`,
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
					Data:  `{"test": {"num_bytes": "32"}}`,
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
		"$ref": "./schema.json#/$defs/Key"
    }
  }
}`,
			Tests: []test{
				{
					Name:  "shared key",
					Valid: true,
					Data:  `{"test": {"shared_key": {"value": "my-shared-key"}}}`,
				},
				{
					Name:  "public/private key",
					Valid: true,
					Data:  `{"test": {"public_key": {"path": "/keys/pub"}, "private_key": {"path": "/keys/priv"}}}`,
				},
				{
					Name:  "shared key with env var",
					Valid: true,
					Data:  `{"test": {"shared_key": {"env_var": "MY_KEY"}}}`,
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
					Data:  `{"test": {"accept_tos": true, "email": "admin@example.com", "cache_dir": "/certs"}}`,
				},
				{
					Name:  "lets encrypt with host whitelist",
					Valid: true,
					Data:  `{"test": {"accept_tos": true, "email": "admin@example.com", "cache_dir": "/certs", "host_whitelist": ["example.com"]}}`,
				},
				{
					Name:  "lets encrypt with renew_before",
					Valid: true,
					Data:  `{"test": {"accept_tos": true, "email": "admin@example.com", "cache_dir": "/certs", "renew_before": "30d"}}`,
				},
				{
					Name:  "self-signed autogen",
					Valid: true,
					Data:  `{"test": {"auto_gen_path": "/certs/autogen"}}`,
				},
				{
					Name:  "cert without key",
					Valid: false,
					Data:  `{"test": {"cert": {"path": "/certs/cert.pem"}}}`,
				},
				{
					Name:  "lets encrypt without accept_tos",
					Valid: false,
					Data:  `{"test": {"email": "admin@example.com", "cache_dir": "/certs"}}`,
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
					Name:  "sqlite with auto_migrate",
					Valid: true,
					Data:  `{"test": {"provider": "sqlite", "path": "/data/db.sqlite", "auto_migrate": true}}`,
				},
				{
					Name:  "sqlite with auto_migration_lock_duration",
					Valid: true,
					Data:  `{"test": {"provider": "sqlite", "path": "/data/db.sqlite", "auto_migration_lock_duration": "30s"}}`,
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
					Data:  `{"test": {"type": "tint", "to": "stderr", "level": "warn", "source": false, "no_color": true, "time_format": "15:04:05"}}`,
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
					Data:  `{"test": {"type": "text", "no_color": true}}`,
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
					Data:  `{"test": {"provider": "redis", "address": "localhost:6379", "network": "tcp", "protocol": 2, "username": "user", "password": {"env_var": "REDIS_PASS"}, "db": 1}}`,
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
					Data:  `{"test": {"external_id": "actor-1", "key": {"shared_key": {"value": "secret"}}}}`,
				},
				{
					Name:  "with permissions",
					Valid: true,
					Data:  `{"test": {"external_id": "actor-1", "key": {"shared_key": {"value": "secret"}}, "permissions": [{"namespace": "root", "resources": ["*"], "verbs": ["*"]}]}}`,
				},
				{
					Name:  "with labels",
					Valid: true,
					Data:  `{"test": {"external_id": "actor-1", "key": {"shared_key": {"value": "secret"}}, "labels": {"env": "prod"}}}`,
				},
				{
					Name:  "missing external_id",
					Valid: false,
					Data:  `{"test": {"key": {"shared_key": {"value": "secret"}}}}`,
				},
				{
					Name:  "missing key",
					Valid: false,
					Data:  `{"test": {"external_id": "actor-1"}}`,
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
					Data:  `{"test": [{"external_id": "actor-1", "key": {"shared_key": {"value": "secret"}}}]}`,
				},
				{
					Name:  "external source",
					Valid: true,
					Data:  `{"test": {"keys_path": "/keys/actors"}}`,
				},
				{
					Name:  "external source with permissions",
					Valid: true,
					Data:  `{"test": {"keys_path": "/keys/actors", "permissions": [{"namespace": "root.**", "resources": ["*"], "verbs": ["*"]}]}}`,
				},
				{
					Name:  "external source with sync cron",
					Valid: true,
					Data:  `{"test": {"keys_path": "/keys/actors", "sync_cron_schedule": "*/5 * * * *"}}`,
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
					Data:  `{"test": {"username": "admin", "key": {"shared_key": {"value": "secret"}}}}`,
				},
				{
					Name:  "with email",
					Valid: true,
					Data:  `{"test": {"username": "admin", "email": "admin@example.com", "key": {"shared_key": {"value": "secret"}}}}`,
				},
				{
					Name:  "with permissions",
					Valid: true,
					Data:  `{"test": {"username": "admin", "key": {"shared_key": {"value": "secret"}}, "permissions": [{"namespace": "root.**", "resources": ["*"], "verbs": ["*"]}]}}`,
				},
				{
					Name:  "missing username",
					Valid: false,
					Data:  `{"test": {"key": {"shared_key": {"value": "secret"}}}}`,
				},
				{
					Name:  "missing key",
					Valid: false,
					Data:  `{"test": {"username": "admin"}}`,
				},
				{
					Name:  "extra property",
					Valid: false,
					Data:  `{"test": {"username": "admin", "key": {"shared_key": {"value": "secret"}}, "extra": "field"}}`,
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
					Data:  `{"test": [{"username": "admin", "key": {"shared_key": {"value": "secret"}}}]}`,
				},
				{
					Name:  "external source",
					Valid: true,
					Data:  `{"test": {"keys_path": "/keys/admins"}}`,
				},
				{
					Name:  "external source with permissions",
					Valid: true,
					Data:  `{"test": {"keys_path": "/keys/admins", "permissions": [{"namespace": "root.**", "resources": ["*"], "verbs": ["*"]}]}}`,
				},
				{
					Name:  "external source with sync cron",
					Valid: true,
					Data:  `{"test": {"keys_path": "/keys/admins", "sync_cron_schedule": "*/5 * * * *"}}`,
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
					Data:  `{"test": {"jwt_signing_key": {"shared_key": {"value": "secret"}}, "jwt_issuer": "my-app"}}`,
				},
				{
					Name:  "jwt_token_duration as integer",
					Valid: true,
					Data:  `{"test": {"jwt_token_duration": 3600000000000}}`,
				},
				{
					Name:  "jwt_token_duration as string is invalid",
					Valid: false,
					Data:  `{"test": {"jwt_token_duration": "1h"}}`,
				},
				{
					Name:  "global_aes_key is KeyData not Key",
					Valid: true,
					Data:  `{"test": {"global_aes_key": {"env_var_base64": "GLOBAL_AES_KEY"}}}`,
				},
				{
					Name:  "actors as external source",
					Valid: true,
					Data:  `{"test": {"actors": {"keys_path": "/keys/actors"}}}`,
				},
				{
					Name:  "actors as inline list",
					Valid: true,
					Data:  `{"test": {"actors": [{"external_id": "svc", "key": {"shared_key": {"value": "secret"}}}]}}`,
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
					Name:  "with identifying_labels",
					Valid: true,
					Data:  `{"test": {"identifying_labels": ["type", "region"]}}`,
				},
				{
					Name:  "with auto_migrate",
					Valid: true,
					Data:  `{"test": {"auto_migrate": true, "auto_migration_lock_duration": "30s"}}`,
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
					Data:  `{"test": {"not_found": "https://example.com/404", "unauthorized": "https://example.com/401"}}`,
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
					Data:  `{"test": {"template": {"env_var": "ERROR_TEMPLATE"}}}`,
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
					Data:  `{"test": {"base_url": "http://localhost:5173"}}`,
				},
				{
					Name:  "base_url as env var (StringValue)",
					Valid: true,
					Data:  `{"test": {"base_url": {"env_var": "MARKETPLACE_URL"}}}`,
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
					Data:  `{"test": {"enabled": true, "base_url": "http://localhost:5174"}}`,
				},
				{
					Name:  "base_url as env var (StringValue)",
					Valid: true,
					Data:  `{"test": {"enabled": true, "base_url": {"env_var": "ADMIN_UI_URL"}}}`,
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
					Data:  `{"test": {"port": {"env_var": "API_PORT"}}}`,
				},
				{
					Name:  "health_check_port as number",
					Valid: true,
					Data:  `{"test": {"port": 8080, "health_check_port": 8081}}`,
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
					Data:  `{"test": {"initiate_session_url": "http://localhost:8888/login"}}`,
				},
				{
					Name:  "empty is valid",
					Valid: true,
					Data:  `{"test": {}}`,
				},
				{
					Name:  "extra property not allowed",
					Valid: false,
					Data:  `{"test": {"initiate_session_url": "http://localhost:8888/login", "extra": "field"}}`,
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
