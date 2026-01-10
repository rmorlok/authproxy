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
