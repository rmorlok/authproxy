package config

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"
)

func Test_SchemaAgainstRealData(t *testing.T) {
	schema, err := compileSchema()
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

			data, err := yamlBytesToJSON(b)
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
