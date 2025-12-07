package common

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

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

func TestSchema(t *testing.T) {
	entities := []entities{
		{
			Name: "UUID",
			Schema: `
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/config/common/test.json",
  "type": "object",
  "additionalProperties": false,
  "required": ["test"],
  "properties": {
	"test": {
		"$ref": "./schema.json#/$defs/UUID"
    }
  }
}`,
			Tests: []test{
				{
					Name:  "bad value",
					Valid: false,
					Data:  `{"test": "bad"}`,
				},
				{
					Name:  "wrong type",
					Valid: false,
					Data:  `{"test": 99}`,
				},
				{
					Name:  "too short 1",
					Valid: false,
					Data:  `{"test": "e8f7dda-7b82-476c-99ff-37c8357da4ae"}`,
				},
				{
					Name:  "too short 2",
					Valid: false,
					Data:  `{"test": "e8f7ddda-7b2-476c-99ff-37c8357da4ae"}`,
				},
				{
					Name:  "too short 3",
					Valid: false,
					Data:  `{"test": "e8f7ddda-7b82-46c-99ff-37c8357da4ae"}`,
				},
				{
					Name:  "too short 4",
					Valid: false,
					Data:  `{"test": "e8f7ddda-7b82-476c-99f-37c8357da4ae"}`,
				},
				{
					Name:  "too short 5",
					Valid: false,
					Data:  `{"test": "e8f7ddda-7b82-476c-99ff-37c835da4ae"}`,
				},
				{
					Name:  "uppercase",
					Valid: true,
					Data:  `{"test": "E8F7DDDA-7B82-476C-99FF-37C8357DA4AE"}`,
				},
				{
					Name:  "lowercase",
					Valid: true,
					Data:  `{"test": "e8f7ddda-7b82-476c-99ff-37c8357da4ae"}`,
				},
				{
					Name:  "no dashes",
					Valid: true,
					Data:  `{"test": "e8f7ddda7b82476c99ff37c8357da4ae"}`,
				},
			},
		},
		{
			Name: "Cron",
			Schema: `
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/config/common/test.json",
  "type": "object",
  "additionalProperties": false,
  "required": ["test"],
  "properties": {
	"test": {
		"$ref": "./schema.json#/$defs/Cron"
    }
  }
}`,
			Tests: []test{
				// it's really hard to have a pattern that actually checks this. Right now we are just treating
				// as a string.
				//{
				//	Name:  "bad value",
				//	Valid: false,
				//	Data:  `{"test": "bad"}`,
				//},
				{
					Name:  "wrong type",
					Valid: false,
					Data:  `{"test": 99}`,
				},
				{
					Name:  "every minute",
					Valid: true,
					Data:  `{"test": "* * * * *"}`,
				},
				{
					Name:  "every 5 minutes",
					Valid: true,
					Data:  `{"test": "*/5 * * * *"}`,
				},
				{
					Name:  "every 10 minutes between 9 AM and 5 PM",
					Valid: true,
					Data:  `{"test": "*/10 9-17 * * *"}`,
				},
				{
					Name:  "every two days at midnight",
					Valid: true,
					Data:  `{"test": "0 0 */2 * *"}`,
				},
			},
		},
		{
			Name: "HumanDuration",
			Schema: `
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/config/common/test.json",
  "type": "object",
  "additionalProperties": false,
  "required": ["test"],
  "properties": {
	"test": {
		"$ref": "./schema.json#/$defs/HumanDuration"
    }
  }
}`,
			Tests: []test{
				{
					Name:  "bad value",
					Valid: false,
					Data:  `{"test": "bad"}`,
				},
				{
					Name:  "wrong type",
					Valid: false,
					Data:  `{"test": 99}`,
				},
				{
					Name:  "no unit",
					Valid: false,
					Data:  `{"test": "99"}`,
				},
				{
					Name:  "only unit",
					Valid: false,
					Data:  `{"test": "ms"}`,
				},
				{
					Name:  "millisecond",
					Valid: true,
					Data:  `{"test": "999ms"}`,
				},
				{
					Name:  "second",
					Valid: true,
					Data:  `{"test": "10s"}`,
				},
				{
					Name:  "minute",
					Valid: true,
					Data:  `{"test": "200m"}`,
				},
				{
					Name:  "hour",
					Valid: true,
					Data:  `{"test": "1h"}`,
				},
				{
					Name:  "day",
					Valid: true,
					Data:  `{"test": "30d"}`,
				},
			},
		},
		{
			Name: "HumanByteSize",
			Schema: `
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/config/common/test.json",
  "type": "object",
  "additionalProperties": false,
  "required": ["test"],
  "properties": {
	"test": {
		"$ref": "./schema.json#/$defs/HumanByteSize"
    }
  }
}`,
			Tests: []test{
				{
					Name:  "bad value",
					Valid: false,
					Data:  `{"test": "bad"}`,
				},
				{
					Name:  "wrong type",
					Valid: false,
					Data:  `{"test": 99}`,
				},
				{
					Name:  "no unit",
					Valid: false,
					Data:  `{"test": "99"}`,
				},
				{
					Name:  "only unit",
					Valid: false,
					Data:  `{"test": "mb"}`,
				},
				{
					Name:  "byte",
					Valid: true,
					Data:  `{"test": "10b"}`,
				},
				{
					Name:  "kilobyte",
					Valid: true,
					Data:  `{"test": "23kb"}`,
				},
				{
					Name:  "EIC kilobyte",
					Valid: true,
					Data:  `{"test": "23kib"}`,
				},
				{
					Name:  "megabyte",
					Valid: true,
					Data:  `{"test": "23mb"}`,
				},
				{
					Name:  "EIC megabyte",
					Valid: true,
					Data:  `{"test": "23mib"}`,
				},
				{
					Name:  "gigabyte",
					Valid: true,
					Data:  `{"test": "23gb"}`,
				},
				{
					Name:  "EIC gigabyte",
					Valid: true,
					Data:  `{"test": "23gib"}`,
				},
				{
					Name:  "permits space",
					Valid: true,
					Data:  `{"test": "23 mb"}`,
				},
				{
					Name:  "case insensitive",
					Valid: true,
					Data:  `{"test": "23KB"}`,
				},
			},
		},
		{
			Name: "Image",
			Schema: `
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/config/common/test.json",
  "type": "object",
  "additionalProperties": false,
  "required": ["test"],
  "properties": {
	"test": {
		"$ref": "./schema.json#/$defs/Image"
    }
  }
}`,
			Tests: []test{
				{
					Name:  "missing properties",
					Valid: false,
					Data:  `{"test": {}}`,
				},
				{
					Name:  "unknown properties",
					Valid: false,
					Data:  `{"test": {"foo": "bar"}}`,
				},
				{
					Name:  "wrong type",
					Valid: false,
					Data:  `{"test": 99}`,
				},
				{
					Name:  "string is public url",
					Valid: true,
					Data:  `{"test": "https://example.com/image.png"}`,
				},
				{
					Name:  "public url",
					Valid: true,
					Data:  `{"test": {"public_url": "https://example.com/image.png"}}`,
				},
				{
					Name:  "public url - other attributes",
					Valid: false,
					Data:  `{"test": {"public_url": "https://example.com/image.png", "other": "value"}}`,
				},
				{
					Name:  "base64",
					Valid: true,
					Data:  `{"test": {"base64": "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+ip1sAAAAASUVORK5CYII=", "mime_type": "image/png"}}`,
				},
				{
					Name:  "base64 - other attributes",
					Valid: false,
					Data:  `{"test": {"base64": "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+ip1sAAAAASUVORK5CYII=", "mime_type": "image/png", "other": "value"}}`,
				},
				{
					Name:  "base64 - missing mime type",
					Valid: false,
					Data:  `{"test": {"base64": "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+ip1sAAAAASUVORK5CYII="}}`,
				},
				{
					Name:  "base64 - missing data",
					Valid: false,
					Data:  `{"test": {"mime_type": "image/png"}}`,
				},
			},
		},
		{
			Name: "StringValue",
			Schema: `
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/config/common/test.json",
  "type": "object",
  "additionalProperties": false,
  "required": ["test"],
  "properties": {
	"test": {
		"$ref": "./schema.json#/$defs/StringValue"
    }
  }
}`,
			Tests: []test{
				{
					Name:  "missing properties",
					Valid: false,
					Data:  `{"test": {}}`,
				},
				{
					Name:  "unknown properties",
					Valid: false,
					Data:  `{"test": {"foo": "bar"}}`,
				},
				{
					Name:  "number",
					Valid: true,
					Data:  `{"test": 99}`,
				},
				{
					Name:  "boolean",
					Valid: true,
					Data:  `{"test": true}`,
				},
				{
					Name:  "string",
					Valid: true,
					Data:  `{"test": "some string"}`,
				},
				{
					Name:  "value",
					Valid: true,
					Data:  `{"test": {"value": "some string"}}`,
				},
				{
					Name:  "value - doesn't coerce number to string",
					Valid: false,
					Data:  `{"test": {"value": 8080}}`,
				},
				{
					Name:  "value - doesn't coerce bool to string",
					Valid: false,
					Data:  `{"test": {"value": true}}`,
				},
				{
					Name:  "value - other attributes",
					Valid: false,
					Data:  `{"test": {"value": "some string", "other": "value"}}`,
				},
				{
					Name:  "base64",
					Valid: true,
					Data:  `{"test": {"base64": "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+ip1sAAAAASUVORK5CYII="}}`,
				},
				{
					Name:  "base64 - other attributes",
					Valid: false,
					Data:  `{"test": {"base64": "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+ip1sAAAAASUVORK5CYII=", "other": "value"}}`,
				},
				{
					Name:  "env var",
					Valid: true,
					Data:  `{"test": {"env_var": "SOME_ENV_VAR"}}`,
				},
				{
					Name:  "env var - default",
					Valid: true,
					Data:  `{"test": {"env_var": "SOME_ENV_VAR", "default": "some string"}}`,
				},
				{
					Name:  "env var - default - doesn't coerce number to string",
					Valid: false,
					Data:  `{"test": {"env_var": "SOME_ENV_VAR", "default": 8080}}`,
				},
				{
					Name:  "env var - default - doesn't coerce bool to string",
					Valid: false,
					Data:  `{"test": {"env_var": "SOME_ENV_VAR", "default": true}}`,
				},
				{
					Name:  "env var - other attributes",
					Valid: false,
					Data:  `{"test": {"env_var": "SOME_ENV_VAR", "default": "some string", "other": "value"}}`,
				},
				{
					Name:  "env var base64",
					Valid: true,
					Data:  `{"test": {"env_var_base64": "SOME_ENV_VAR"}}`,
				},
				{
					Name:  "env var base64 - default",
					Valid: true,
					Data:  `{"test": {"env_var_base64": "SOME_ENV_VAR", "default": "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+ip1sAAAAASUVORK5CYII="}}`,
				},
				{
					Name:  "env var base64 - other attributes",
					Valid: false,
					Data:  `{"test": {"env_var_base64": "SOME_ENV_VAR", "default": "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+ip1sAAAAASUVORK5CYII=", "other": "value"}}`,
				},
				{
					Name:  "file",
					Valid: true,
					Data:  `{"test": {"path": "/path/to/file"}}`,
				},
				{
					Name:  "file - other attributes",
					Valid: false,
					Data:  `{"test": {"path": "/path/to/file", "other": "value"}}`,
				},
			},
		},
		{
			Name: "StringValueNumber",
			Schema: `
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/config/common/test.json",
  "type": "object",
  "additionalProperties": false,
  "required": ["test"],
  "properties": {
	"test": {
		"$ref": "./schema.json#/$defs/StringValueNumber"
    }
  }
}`,
			Tests: []test{
				{
					Name:  "missing properties",
					Valid: false,
					Data:  `{"test": {}}`,
				},
				{
					Name:  "unknown properties",
					Valid: false,
					Data:  `{"test": {"foo": "bar"}}`,
				},
				{
					Name:  "number",
					Valid: true,
					Data:  `{"test": 99}`,
				},
				{
					Name:  "boolean",
					Valid: false,
					Data:  `{"test": true}`,
				},
				{
					Name:  "string",
					Valid: false,
					Data:  `{"test": "some string"}`,
				},
				{
					Name:  "value",
					Valid: true,
					Data:  `{"test": {"value": "some string"}}`,
				},
				{
					Name:  "value - doesn't coerce number to string",
					Valid: false,
					Data:  `{"test": {"value": 8080}}`,
				},
				{
					Name:  "value - doesn't coerce bool to string",
					Valid: false,
					Data:  `{"test": {"value": true}}`,
				},
				{
					Name:  "value - other attributes",
					Valid: false,
					Data:  `{"test": {"value": "some string", "other": "value"}}`,
				},
				{
					Name:  "base64",
					Valid: true,
					Data:  `{"test": {"base64": "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+ip1sAAAAASUVORK5CYII="}}`,
				},
				{
					Name:  "base64 - other attributes",
					Valid: false,
					Data:  `{"test": {"base64": "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+ip1sAAAAASUVORK5CYII=", "other": "value"}}`,
				},
				{
					Name:  "env var",
					Valid: true,
					Data:  `{"test": {"env_var": "SOME_ENV_VAR"}}`,
				},
				{
					Name:  "env var - default",
					Valid: true,
					Data:  `{"test": {"env_var": "SOME_ENV_VAR", "default": "some string"}}`,
				},
				{
					Name:  "env var - default - doesn't coerce number to string",
					Valid: false,
					Data:  `{"test": {"env_var": "SOME_ENV_VAR", "default": 8080}}`,
				},
				{
					Name:  "env var - default - doesn't coerce bool to string",
					Valid: false,
					Data:  `{"test": {"env_var": "SOME_ENV_VAR", "default": true}}`,
				},
				{
					Name:  "env var - other attributes",
					Valid: false,
					Data:  `{"test": {"env_var": "SOME_ENV_VAR", "default": "some string", "other": "value"}}`,
				},
				{
					Name:  "env var base64",
					Valid: true,
					Data:  `{"test": {"env_var_base64": "SOME_ENV_VAR"}}`,
				},
				{
					Name:  "env var base64 - default",
					Valid: true,
					Data:  `{"test": {"env_var_base64": "SOME_ENV_VAR", "default": "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+ip1sAAAAASUVORK5CYII="}}`,
				},
				{
					Name:  "env var base64 - other attributes",
					Valid: false,
					Data:  `{"test": {"env_var_base64": "SOME_ENV_VAR", "default": "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+ip1sAAAAASUVORK5CYII=", "other": "value"}}`,
				},
				{
					Name:  "file",
					Valid: true,
					Data:  `{"test": {"path": "/path/to/file"}}`,
				},
				{
					Name:  "file - other attributes",
					Valid: false,
					Data:  `{"test": {"path": "/path/to/file", "other": "value"}}`,
				},
			},
		},
		{
			Name: "StringValueBool",
			Schema: `
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/config/common/test.json",
  "type": "object",
  "additionalProperties": false,
  "required": ["test"],
  "properties": {
	"test": {
		"$ref": "./schema.json#/$defs/StringValueBool"
    }
  }
}`,
			Tests: []test{
				{
					Name:  "missing properties",
					Valid: false,
					Data:  `{"test": {}}`,
				},
				{
					Name:  "unknown properties",
					Valid: false,
					Data:  `{"test": {"foo": "bar"}}`,
				},
				{
					Name:  "number",
					Valid: false,
					Data:  `{"test": 99}`,
				},
				{
					Name:  "boolean",
					Valid: true,
					Data:  `{"test": true}`,
				},
				{
					Name:  "string",
					Valid: false,
					Data:  `{"test": "some string"}`,
				},
				{
					Name:  "value",
					Valid: true,
					Data:  `{"test": {"value": "some string"}}`,
				},
				{
					Name:  "value - doesn't coerce number to string",
					Valid: false,
					Data:  `{"test": {"value": 8080}}`,
				},
				{
					Name:  "value - doesn't coerce bool to string",
					Valid: false,
					Data:  `{"test": {"value": true}}`,
				},
				{
					Name:  "value - other attributes",
					Valid: false,
					Data:  `{"test": {"value": "some string", "other": "value"}}`,
				},
				{
					Name:  "base64",
					Valid: true,
					Data:  `{"test": {"base64": "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+ip1sAAAAASUVORK5CYII="}}`,
				},
				{
					Name:  "base64 - other attributes",
					Valid: false,
					Data:  `{"test": {"base64": "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+ip1sAAAAASUVORK5CYII=", "other": "value"}}`,
				},
				{
					Name:  "env var",
					Valid: true,
					Data:  `{"test": {"env_var": "SOME_ENV_VAR"}}`,
				},
				{
					Name:  "env var - default",
					Valid: true,
					Data:  `{"test": {"env_var": "SOME_ENV_VAR", "default": "some string"}}`,
				},
				{
					Name:  "env var - default - doesn't coerce number to string",
					Valid: false,
					Data:  `{"test": {"env_var": "SOME_ENV_VAR", "default": 8080}}`,
				},
				{
					Name:  "env var - default - doesn't coerce bool to string",
					Valid: false,
					Data:  `{"test": {"env_var": "SOME_ENV_VAR", "default": true}}`,
				},
				{
					Name:  "env var - other attributes",
					Valid: false,
					Data:  `{"test": {"env_var": "SOME_ENV_VAR", "default": "some string", "other": "value"}}`,
				},
				{
					Name:  "env var base64",
					Valid: true,
					Data:  `{"test": {"env_var_base64": "SOME_ENV_VAR"}}`,
				},
				{
					Name:  "env var base64 - default",
					Valid: true,
					Data:  `{"test": {"env_var_base64": "SOME_ENV_VAR", "default": "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+ip1sAAAAASUVORK5CYII="}}`,
				},
				{
					Name:  "env var base64 - other attributes",
					Valid: false,
					Data:  `{"test": {"env_var_base64": "SOME_ENV_VAR", "default": "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+ip1sAAAAASUVORK5CYII=", "other": "value"}}`,
				},
				{
					Name:  "file",
					Valid: true,
					Data:  `{"test": {"path": "/path/to/file"}}`,
				},
				{
					Name:  "file - other attributes",
					Valid: false,
					Data:  `{"test": {"path": "/path/to/file", "other": "value"}}`,
				},
			},
		},
		{
			Name: "NamespacePath",
			Schema: `
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/config/common/test.json",
  "type": "object",
  "additionalProperties": false,
  "required": ["test"],
  "properties": {
	"test": {
		"$ref": "./schema.json#/$defs/NamespacePath"
    }
  }
}`,
			Tests: []test{
				{
					Name:  "bad value",
					Valid: false,
					Data:  `{"test": "bad"}`,
				},
				{
					Name:  "wrong type",
					Valid: false,
					Data:  `{"test": 99}`,
				},
				{
					Name:  "empty",
					Valid: false,
					Data:  `{"test": ""}`,
				},
				{
					Name:  "not rooted",
					Valid: false,
					Data:  `{"test": "other/namespace"}`,
				},
				{
					Name:  "trailing dot",
					Valid: false,
					Data:  `{"test": "root."}`,
				},
				{
					Name:  "case sensitive",
					Valid: false,
					Data:  `{"test": "ROOT"}`,
				},
				{
					Name:  "nested trailing dot",
					Valid: false,
					Data:  `{"test": "root.other."}`,
				},
				{
					Name:  "cannot start with dash",
					Valid: false,
					Data:  `{"test": "root.-other"}`,
				},
				{
					Name:  "root",
					Valid: true,
					Data:  `{"test": "root"}`,
				},
				{
					Name:  "nested",
					Valid: true,
					Data:  `{"test": "root.other"}`,
				},
				{
					Name:  "deeply nested",
					Valid: true,
					Data:  `{"test": "root.foo.bar.baz"}`,
				},
				{
					Name:  "allows underscores",
					Valid: true,
					Data:  `{"test": "root.foo_bar"}`,
				},
				{
					Name:  "can start with underscore",
					Valid: true,
					Data:  `{"test": "root._foo"}`,
				},
				{
					Name:  "allows dashes",
					Valid: true,
					Data:  `{"test": "root.foo-bar"}`,
				},
				{
					Name:  "allows just numbers",
					Valid: true,
					Data:  `{"test": "root.1234"}`,
				},
				{
					Name:  "allows mixed",
					Valid: true,
					Data:  `{"test": "root.foo-1234_bar"}`,
				},
			},
		},
	}

	for _, entity := range entities {
		t.Run(entity.Name, func(t *testing.T) {
			c := jsonschemav5.NewCompiler()

			_ = loadSchema(t, c, "./schema.json")

			err := c.AddResource(
				"https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/config/common/test.json",
				strings.NewReader(strings.TrimSpace(entity.Schema)),
			)
			require.NoError(t, err)

			schema, err := c.Compile("https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/config/common/test.json")
			require.NoError(t, err)

			for _, test := range entity.Tests {
				t.Run(test.Name, func(t *testing.T) {
					var v interface{}
					if err := json.Unmarshal([]byte(test.Data), &v); err != nil {
						t.Fatalf("failed to unmarshal JSON: %v", err)
					}

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
