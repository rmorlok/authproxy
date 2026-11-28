package apply

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/rmorlok/authproxy/internal/apserde"
	"github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
	"gopkg.in/yaml.v3"
)

// LoadToolSetDefinition reads one YAML or JSON definition from a regular local
// file or stdin ("-"). The input contains the fields of spec.definition only.
func LoadToolSetDefinition(filename string, stdin io.Reader) (*toolsets.ToolSetDefinition, error) {
	if filename == "" || strings.Contains(filename, "://") {
		return nil, fmt.Errorf("--filename must be a local definition file or - for stdin")
	}
	if filename == "-" {
		if stdin == nil {
			return nil, fmt.Errorf("stdin is not available")
		}
		return readToolSetDefinition("stdin", stdin)
	}

	// Reject devices and named pipes before opening them. Check the opened
	// file as well so the mode check describes the handle we read.
	info, err := os.Stat(filename)
	if err != nil {
		return nil, fmt.Errorf("%s: cannot access definition file: %w", filename, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: expected a regular definition file", filename)
	}
	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("%s: cannot open definition file: %w", filename, err)
	}
	defer f.Close()

	info, err = f.Stat()
	if err != nil {
		return nil, fmt.Errorf("%s: cannot inspect definition file: %w", filename, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: expected a regular definition file", filename)
	}
	return readToolSetDefinition(filename, f)
}

// readToolSetDefinition checks stream size and YAML key structure before
// using the canonical definition codec. Decoder errors can embed input
// values, so return the source and stage rather than the codec's error text.
func readToolSetDefinition(source string, r io.Reader) (*toolsets.ToolSetDefinition, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxSourceBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%s: failed to read ToolSet definition", source)
	}
	if len(data) > maxSourceBytes {
		return nil, fmt.Errorf("%s: exceeds 16 MiB definition limit", source)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var node yaml.Node
	if err := decoder.Decode(&node); err == io.EOF {
		return nil, fmt.Errorf("%s: expected one ToolSet definition object", source)
	} else if err != nil {
		return nil, fmt.Errorf("%s: malformed YAML or JSON", source)
	}
	if err := decoder.Decode(&yaml.Node{}); err == nil {
		return nil, fmt.Errorf("%s: expected exactly one ToolSet definition document", source)
	} else if err != io.EOF {
		return nil, fmt.Errorf("%s: malformed YAML or JSON", source)
	}
	if err := checkNode(&node); err != nil {
		return nil, fmt.Errorf("%s: %w", source, err)
	}
	if len(node.Content) != 1 || node.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s: expected one ToolSet definition object", source)
	}

	var definition toolsets.ToolSetDefinition
	if err := node.Decode(&definition); err != nil {
		return nil, fmt.Errorf("%s: invalid ToolSet definition fields or types (expected the contents of spec.definition)", source)
	}
	if err := validateToolSetDefinition(&definition); err != nil {
		return nil, fmt.Errorf("%s: %w", source, err)
	}
	return &definition, nil
}

// validateToolSetDefinition checks the authored replacement before it can be
// submitted. Keep diagnostics free of authored values.
func validateToolSetDefinition(definition *toolsets.ToolSetDefinition) error {
	if err := apserde.ValidateNoRedactedPlaceholders(definition); err != nil {
		return fmt.Errorf("redacted placeholders cannot be submitted")
	}
	if err := definition.Validate(nil); err != nil {
		return fmt.Errorf("ToolSet definition failed semantic validation")
	}
	return nil
}
