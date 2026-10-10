package common

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// RawJSON holds arbitrary JSON data that can be deserialized from both YAML and JSON.
// When unmarshaled from YAML, it converts the YAML structure to its JSON representation.
// When marshaled, it outputs raw JSON bytes.
type RawJSON json.RawMessage

// MarshalJSON returns the raw JSON bytes.
func (r RawJSON) MarshalJSON() ([]byte, error) {
	if r == nil {
		return []byte("null"), nil
	}
	return json.RawMessage(r).MarshalJSON()
}

// UnmarshalJSON stores the raw JSON bytes.
func (r *RawJSON) UnmarshalJSON(data []byte) error {
	rm := json.RawMessage(data)
	*r = RawJSON(rm)
	return nil
}

// UnmarshalYAML converts YAML data to JSON bytes for storage. Mapping keys are
// emitted as their scalar text, so unquoted keys such as OpenAPI status codes
// (200:) become JSON object keys. Integer and float scalars whose text is
// already a JSON number are copied verbatim, keeping large integers and
// authored precision; other numeric spellings use yaml.v3's native decoding.
func (r *RawJSON) UnmarshalYAML(value *yaml.Node) error {
	// Native decoding is retained as a validation pass: it rejects duplicate
	// keys, recursive anchors, and excessive alias expansion before conversion.
	var native interface{}
	if err := value.Decode(&native); err != nil {
		return err
	}

	converted, err := yamlNodeJSONValue(value)
	if err != nil {
		return err
	}

	jsonBytes, err := json.Marshal(converted)
	if err != nil {
		return err
	}
	*r = RawJSON(jsonBytes)
	return nil
}

// jsonNumberText matches the JSON number grammar, which is the subset of YAML
// numeric scalars that can be stored without reinterpreting the authored text.
var jsonNumberText = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?$`)

// yamlNodeJSONValue converts a YAML node into a value json.Marshal accepts.
// Aliases are expanded, merge keys are applied with explicit keys taking
// precedence, and keys that collide after conversion to text are rejected.
func yamlNodeJSONValue(node *yaml.Node) (any, error) {
	switch node.Kind {
	case 0:
		return nil, nil
	case yaml.DocumentNode:
		if len(node.Content) == 0 {
			return nil, nil
		}
		return yamlNodeJSONValue(node.Content[0])
	case yaml.AliasNode:
		return yamlNodeJSONValue(node.Alias)
	case yaml.SequenceNode:
		items := make([]any, 0, len(node.Content))
		for _, child := range node.Content {
			item, err := yamlNodeJSONValue(child)
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		return items, nil
	case yaml.MappingNode:
		return yamlMappingJSONValue(node)
	case yaml.ScalarNode:
		tag := node.ShortTag()
		if (tag == "!!int" || tag == "!!float") &&
			jsonNumberText.MatchString(node.Value) {
			return json.Number(node.Value), nil
		}

		var scalar any
		if err := node.Decode(&scalar); err != nil {
			return nil, err
		}
		return scalar, nil
	default:
		return nil, fmt.Errorf("unsupported YAML node kind %d", node.Kind)
	}
}

// yamlMappingJSONValue converts a mapping to a string-keyed object. Merged
// mappings fill keys not set explicitly; earlier merge sources take precedence.
func yamlMappingJSONValue(node *yaml.Node) (map[string]any, error) {
	object := make(map[string]any, len(node.Content)/2)
	merged := make(map[string]any)

	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode, valueNode := resolveYAMLAlias(node.Content[i]), node.Content[i+1]

		if keyNode.Kind == yaml.ScalarNode && keyNode.ShortTag() == "!!merge" {
			if err := mergeYAMLMapping(merged, valueNode); err != nil {
				return nil, err
			}
			continue
		}

		if keyNode.Kind != yaml.ScalarNode {
			return nil, fmt.Errorf("line %d: YAML mapping keys must be scalars", keyNode.Line)
		}
		if _, exists := object[keyNode.Value]; exists {
			return nil, fmt.Errorf("line %d: mapping key %q is defined more than once", keyNode.Line, keyNode.Value)
		}

		value, err := yamlNodeJSONValue(valueNode)
		if err != nil {
			return nil, err
		}

		object[keyNode.Value] = value
	}

	for key, value := range merged {
		if _, exists := object[key]; !exists {
			object[key] = value
		}
	}

	return object, nil
}

// mergeYAMLMapping applies a << value, which is a mapping or a sequence of
// mappings, adding only keys that an earlier merge source has not supplied.
func mergeYAMLMapping(merged map[string]any, source *yaml.Node) error {
	source = resolveYAMLAlias(source)

	sources := []*yaml.Node{source}
	if source.Kind == yaml.SequenceNode {
		sources = source.Content
	}

	for _, item := range sources {
		value, err := yamlNodeJSONValue(item)
		if err != nil {
			return err
		}

		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("line %d: merge values must be mappings", item.Line)
		}

		for key, entry := range object {
			if _, exists := merged[key]; !exists {
				merged[key] = entry
			}
		}
	}

	return nil
}

// resolveYAMLAlias follows alias nodes to the anchored node they reference.
func resolveYAMLAlias(node *yaml.Node) *yaml.Node {
	for node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	return node
}

// MarshalYAML emits structured YAML without decoding numbers through float64.
// Keeping numeric nodes intact avoids rounding large integer schema constraints
// or payload values merely by exporting a resource as YAML. This preserves
// export precision, and UnmarshalYAML copies JSON-compatible numbers verbatim.
func (r RawJSON) MarshalYAML() (interface{}, error) {
	if r == nil {
		return nil, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(r))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("invalid raw JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("invalid raw JSON: expected exactly one JSON value")
	}
	return rawJSONYAMLNode(value)
}

// rawJSONYAMLNode converts values from a UseNumber decoder to YAML nodes.
// Explicit numeric tags preserve both the original number text and its type,
// even when yaml.v3's implicit resolver would treat an overflow as a string.
// Sorted object keys make exports deterministic; the encoder chooses quoting
// for ordinary scalars and readable block formatting for containers.
func rawJSONYAMLNode(value any) (*yaml.Node, error) {
	switch value := value.(type) {
	case json.Number:
		tag := "!!int"
		if strings.ContainsAny(value.String(), ".eE") {
			tag = "!!float"
		}
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value.String()}, nil
	case map[string]any:
		node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			child, err := rawJSONYAMLNode(value[key])
			if err != nil {
				return nil, err
			}
			node.Content = append(
				node.Content,
				&yaml.Node{
					Kind:  yaml.ScalarNode,
					Tag:   "!!str",
					Value: key},
				child,
			)
		}
		return node, nil
	case []any:
		node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, item := range value {
			child, err := rawJSONYAMLNode(item)
			if err != nil {
				return nil, err
			}
			node.Content = append(node.Content, child)
		}
		return node, nil
	default:
		var node yaml.Node
		if err := node.Encode(value); err != nil {
			return nil, err
		}
		return &node, nil
	}
}

// IsEmpty returns true if the raw JSON is nil or empty.
func (r RawJSON) IsEmpty() bool {
	return len(r) == 0
}
