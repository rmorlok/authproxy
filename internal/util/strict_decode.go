package util

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

// DecodeJSONStrict decodes exactly one JSON value and rejects fields that are
// not declared by the destination struct. Callers that intentionally accept
// arbitrary JSON should continue to use json.RawMessage or map[string]any.
func DecodeJSONStrict(data []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected additional JSON value")
		}
		return err
	}

	return nil
}

// DecodeYAMLStrict decodes exactly one YAML document and rejects fields that
// are not declared by the destination struct.
func DecodeYAMLStrict(data []byte, destination any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(destination); err != nil {
		return err
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected additional YAML document")
		}
		return err
	}

	return nil
}

// DecodeYAMLNodeStrict applies DecodeYAMLStrict to a node used by a custom
// yaml.Unmarshaler. It keeps union types from bypassing KnownFields.
func DecodeYAMLNodeStrict(node *yaml.Node, destination any) error {
	data, err := yaml.Marshal(node)
	if err != nil {
		return err
	}
	return DecodeYAMLStrict(data, destination)
}
