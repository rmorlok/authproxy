package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/template"

	"k8s.io/client-go/util/jsonpath"
)

type applyPrinter func(io.Writer, any) error

// Parse before mutations; execute only against sanitized JSON-shaped output.
func newApplyPrinter(format, expression string, allowMissing bool) (applyPrinter, error) {
	if before, after, ok := strings.Cut(format, "="); ok {
		if expression != "" {
			return nil, fmt.Errorf("supply the template only once")
		}
		format, expression = before, after
	}
	switch format {
	case "", "name", "json", "yaml":
		if expression != "" {
			return nil, fmt.Errorf("--template requires a template or JSONPath output format")
		}
		return nil, nil
	case "go-template", "go-template-file", "jsonpath", "jsonpath-file", "jsonpath-as-json":
	default:
		return nil, fmt.Errorf("unsupported output format %q", format)
	}
	if expression == "" {
		return nil, fmt.Errorf("a template expression or file is required")
	}
	if strings.HasSuffix(format, "-file") {
		data, err := os.ReadFile(expression)
		if err != nil {
			return nil, fmt.Errorf("cannot read template file: %w", err)
		}
		expression = string(data)
	}
	var execute func(io.Writer, any) error
	if strings.HasPrefix(format, "go-template") {
		option := "missingkey=error"
		if allowMissing {
			option = "missingkey=default"
		}
		t, err := template.New("apply").Option(option).Parse(expression)
		if err != nil {
			return nil, fmt.Errorf("invalid Go template: %w", err)
		}
		execute = t.Execute
	} else {
		p := jsonpath.New("apply").AllowMissingKeys(allowMissing)
		p.EnableJSONOutput(format == "jsonpath-as-json")
		if err := p.Parse(expression); err != nil {
			return nil, fmt.Errorf("invalid JSONPath: %w", err)
		}
		execute = p.Execute
	}
	return func(w io.Writer, value any) error {
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		var object any
		if err = json.Unmarshal(data, &object); err != nil {
			return err
		}
		var out bytes.Buffer
		if err = execute(&out, object); err != nil {
			return fmt.Errorf("cannot render apply output: %w", err)
		}
		_, err = out.WriteTo(w)
		return err
	}, nil
}
