package config

import (
	"bytes"
	"encoding/json"
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
	t.Helper()
	schemaBytes, err := os.ReadFile(path)
	require.NoError(t, err)

	var sid schemaId
	err = json.Unmarshal(schemaBytes, &sid)
	require.NoError(t, err)

	err = c.AddResource(sid.Id, bytes.NewReader(schemaBytes))
	require.NoError(t, err)

	return sid.Id
}

// commonSchemaPath is the relative path from cmd/cli/config to the internal common schema.
const commonSchemaPath = "../../../internal/schema/common/schema.json"

func Test_SchemaAgainstRealData(t *testing.T) {
	c := jsonschemav5.NewCompiler()

	_ = loadSchema(t, c, commonSchemaPath)
	sid := loadSchema(t, c, "./schema.json")

	require.Equal(t, SchemaIdCliConfig, sid, "schema ID should match SchemaIdCliConfig constant")

	schema, err := c.Compile(sid)
	require.NoError(t, err)

	files, err := filepath.Glob("test_data/*.yaml")
	require.NoError(t, err)
	require.NotEmpty(t, files, "no test files found")

	for _, cfgPath := range files {
		name := strings.TrimSuffix(filepath.Base(cfgPath), ".yaml")
		if !strings.HasPrefix(name, "valid") && !strings.HasPrefix(name, "invalid") {
			t.Fatalf("invalid test file name: %s; must start with valid or invalid", name)
		}

		t.Run(name, func(t *testing.T) {
			b, err := os.ReadFile(cfgPath)
			require.NoError(t, err)

			data, err := util.YamlBytesToJSON(b)
			require.NoError(t, err)

			var v interface{}
			require.NoError(t, json.Unmarshal(data, &v))

			err = schema.Validate(v)
			shouldBeValid := strings.HasPrefix(name, "valid")
			if shouldBeValid {
				require.NoErrorf(t, err, "%s should be valid against schema", cfgPath)
			} else {
				require.Errorf(t, err, "%s should not be valid against schema", cfgPath)
			}
		})
	}
}

type schemaTest struct {
	Name  string
	Valid bool
	Data  string
}

func TestSchema(t *testing.T) {
	tests := []struct {
		Def   string
		Cases []schemaTest
	}{
		{
			Def: "Root",
			Cases: []schemaTest{
				{Name: "empty object", Valid: true, Data: `{}`},
				{Name: "string admin_username", Valid: true, Data: `{"admin_username": "bob"}`},
				{Name: "object admin_username via env var", Valid: true, Data: `{"admin_username": {"env_var": "USER"}}`},
				{Name: "object admin_username via env var with default", Valid: true, Data: `{"admin_username": {"env_var": "USER", "default": "bob"}}`},
				{Name: "object admin_username via path", Valid: true, Data: `{"admin_username": {"path": "/etc/user"}}`},
				{Name: "object admin_username via template_env_vars", Valid: true, Data: `{"admin_username": {"template_env_vars": "{{ENV}}-{{USER}}"}}`},
				{Name: "extra root property", Valid: false, Data: `{"unknown": "value"}`},
				{Name: "non-object root", Valid: false, Data: `"not an object"`},
				{Name: "admin_username bad form", Valid: false, Data: `{"admin_username": {"unknown_form": "x"}}`},
				{Name: "server present and well-formed", Valid: true, Data: `{"server": {"api": "http://localhost:8081"}}`},
				{Name: "server with all fields", Valid: true, Data: `{"server": {"api": "http://localhost:8081", "admin_api": "http://localhost:8082", "auth": "http://localhost:8080", "marketplace": "http://localhost:5173", "admin_ui": "http://localhost:5174"}}`},
				{Name: "server extra property rejected", Valid: false, Data: `{"server": {"api": "http://localhost:8081", "bogus": "x"}}`},
				{Name: "server is not an object", Valid: false, Data: `{"server": "http://localhost"}`},
				{Name: "signing_proxy with port string", Valid: true, Data: `{"signing_proxy": {"port": "8898"}}`},
				{Name: "signing_proxy with port via env var", Valid: true, Data: `{"signing_proxy": {"port": {"env_var": "SP_PORT", "default": "8888"}}}`},
				{Name: "signing_proxy extra property rejected", Valid: false, Data: `{"signing_proxy": {"port": "8898", "bogus": "x"}}`},
				{Name: "admin_private_key_path as env var", Valid: true, Data: `{"admin_private_key_path": {"env_var": "ADMIN_KEY_PATH"}}`},
				{Name: "admin_shared_key_path as path", Valid: true, Data: `{"admin_shared_key_path": {"path": "~/.authproxy/shared.key"}}`},
				{Name: "admin_username inline bool coerced", Valid: true, Data: `{"admin_username": true}`},
				{Name: "admin_username inline number coerced", Valid: true, Data: `{"admin_username": 99}`},
				{Name: "admin_username object with both value and env_var rejected", Valid: false, Data: `{"admin_username": {"value": "x", "env_var": "Y"}}`},
				{Name: "admin_username object with value and extra rejected", Valid: false, Data: `{"admin_username": {"value": "x", "extra": "y"}}`},
			},
		},
		{
			Def: "Server",
			Cases: []schemaTest{
				{Name: "empty", Valid: true, Data: `{}`},
				{Name: "api only", Valid: true, Data: `{"api": "http://localhost:8081"}`},
				{Name: "all urls", Valid: true, Data: `{"api": "http://localhost:8081", "admin_api": "http://localhost:8082", "auth": "http://localhost:8080", "marketplace": "http://localhost:5173", "admin_ui": "http://localhost:5174"}`},
				{Name: "api as env var", Valid: true, Data: `{"api": {"env_var": "API_URL", "default": "http://localhost:8081"}}`},
				{Name: "extra property rejected", Valid: false, Data: `{"api": "http://localhost:8081", "extra": "bogus"}`},
				{Name: "api inline bool coerced", Valid: true, Data: `{"api": true}`},
				{Name: "api inline number coerced", Valid: true, Data: `{"api": 99}`},
				{Name: "api object with mixed forms rejected", Valid: false, Data: `{"api": {"value": "x", "path": "/p"}}`},
			},
		},
		{
			Def: "SigningProxy",
			Cases: []schemaTest{
				{Name: "empty", Valid: true, Data: `{}`},
				{Name: "port string", Valid: true, Data: `{"port": "8888"}`},
				{Name: "port via env var with default", Valid: true, Data: `{"port": {"env_var": "AUTHPROXY_SIGNING_PROXY_PORT", "default": "8888"}}`},
				{Name: "port inline number coerced", Valid: true, Data: `{"port": 8888}`},
				{Name: "extra property rejected", Valid: false, Data: `{"port": "8888", "extra": "bogus"}`},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.Def, func(t *testing.T) {
			c := jsonschemav5.NewCompiler()

			_ = loadSchema(t, c, commonSchemaPath)
			sid := loadSchema(t, c, "./schema.json")
			require.Equal(t, SchemaIdCliConfig, sid)

			wrapperId := "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/cli/test-" + tc.Def + ".json"
			wrapper := `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "` + wrapperId + `",
  "type": "object",
  "additionalProperties": false,
  "required": ["test"],
  "properties": {
    "test": { "$ref": "./schema.json#/$defs/` + tc.Def + `" }
  }
}`
			require.NoError(t, c.AddResource(wrapperId, strings.NewReader(wrapper)))

			schema, err := c.Compile(wrapperId)
			require.NoError(t, err)

			for _, tt := range tc.Cases {
				t.Run(tt.Name, func(t *testing.T) {
					var v interface{}
					require.NoError(t, json.Unmarshal([]byte(`{"test": `+tt.Data+`}`), &v))

					err = schema.Validate(v)
					if tt.Valid {
						require.NoError(t, err)
					} else {
						require.Error(t, err)
					}
				})
			}
		})
	}
}
