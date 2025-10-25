package configschema

import (
	"encoding/json"
	"log"
	"os"

	"github.com/invopop/jsonschema"
	"github.com/rmorlok/authproxy/config"
)

//go:generate go run ./internal/configschema/gen.go

// Generate writes the reflected JSON Schema for config.Root to the provided path
// and also returns the pretty-printed JSON bytes for further use.
func Generate(outPath string) ([]byte, error) {
	r := &jsonschema.Reflector{
		ExpandedStruct: true,
		DoNotReference: false,
		// Only treat fields as required when explicitly tagged with `jsonschema:"required"`.
		// This keeps the schema permissive to allow defaulting in code while still validating types.
		RequiredFromJSONSchemaTags: true,
	}

	schema := r.Reflect(&config.Root{})

	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll("docs", 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return nil, err
	}

	return data, nil
}

// This small program generates a JSON Schema for the configuration file structure
// by reflecting the Go structs and their json tags. The generated file is written
// to docs/config.schema.json so it can be referenced by editors and tooling.
func main() {
	outPath := "config/schema.json"
	data, err := Generate(outPath)
	if err != nil {
		log.Fatalf("failed to generate schema: %v", err)
	}

	log.Printf("wrote %s (size=%d bytes)", outPath, len(data))
}
