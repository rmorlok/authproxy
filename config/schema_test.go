package config

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/invopop/jsonschema"
	jsonschemav5 "github.com/santhosh-tekuri/jsonschema/v5"
	"gopkg.in/yaml.v3"
)

// generateSchemaBytes reflects the Root struct into a JSON Schema.
func generateSchemaBytes(t *testing.T) []byte {
	r := &jsonschema.Reflector{
		ExpandedStruct:             true,
		DoNotReference:             false,
		RequiredFromJSONSchemaTags: true,
	}
	s := r.Reflect(&Root{})
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal schema: %v", err)
	}
	// also persist for user reference
	_ = ioutil.WriteFile(filepath.Join("..", "docs", "config.schema.json"), b, 0o644)
	return b
}

// loadYAMLAsJSON loads a YAML file and returns canonical JSON bytes
func loadYAMLAsJSON(t *testing.T, path string) []byte {
	t.Helper()
	b, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	var v interface{}
	if err := yaml.Unmarshal(b, &v); err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}
	// YAML numbers default to int/float; ensure JSON-encodable
	j, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal to JSON: %v", err)
	}
	return j
}

// compileSchema compiles schema bytes with jsonschema/v5
func compileSchema(t *testing.T, schemaBytes []byte) *jsonschemav5.Schema {
	t.Helper()
	c := jsonschemav5.NewCompiler()
	url := "mem://config.schema.json"
	c.AddResource(url, bytes.NewReader(schemaBytes))
	s, err := c.Compile(url)
	if err != nil {
		t.Fatalf("failed to compile schema: %v", err)
	}
	return s
}

func Test_ConfigSchema_ValidatesDevConfigYAML(t *testing.T) {
	schemaBytes := generateSchemaBytes(t)
	schema := compileSchema(t, schemaBytes)

	cfgPath := filepath.Join("..", "dev_config", "default.yaml")
	data := loadYAMLAsJSON(t, cfgPath)
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}
	if err := schema.Validate(v); err != nil {
		t.Fatalf("dev_config/default.yaml should be valid against schema, got error: %v", err)
	}
}

func Test_ConfigSchema_NegativeCase_InvalidBoolean(t *testing.T) {
	schemaBytes := generateSchemaBytes(t)
	schema := compileSchema(t, schemaBytes)

	cfgPath := filepath.Join("..", "dev_config", "default.yaml")
	var doc map[string]interface{}
	b, err := ioutil.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", cfgPath, err)
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}
	// Introduce an invalid type: http_logging.enabled must be a boolean, set to string
	if hl, ok := doc["http_logging"].(map[string]interface{}); ok {
		hl["enabled"] = "yes"
	}
	if err := schema.Validate(doc); err == nil {
		t.Fatalf("expected validation error for invalid type (http_logging.enabled as string), but got none")
	}
}
