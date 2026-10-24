package render

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"
)

// Input contains the explicitly allowed invocation context. Callers validate
// Params and capture configuration/metadata before constructing this snapshot.
// There is deliberately no connection object or credential field to reflect on.
type Input struct {
	Params              map[string]any
	Cfg                 map[string]any
	Labels              map[string]string
	Annotations         map[string]string
	ConnectionID        string
	ConnectionNamespace string
	ToolID              string
	Revision            uint64
	ToolSetID           string
	ToolSetGeneration   *uint64
	CatalogRevision     *uint64
}

// Context owns detached JSON values. Its contents are private and immutable
// after construction, so independent renders can safely share the snapshot.
type Context struct {
	root map[string]any
}

// NewContext copies only JSON-compatible data, preserving numeric precision.
// Structs, pointers, custom marshalers, functions, and other Go objects are
// rejected instead of exposing their fields or methods to templates. Cycles
// fail the depth limit. Errors never include configuration values.
func NewContext(input Input, limits Limits) (*Context, error) {
	if err := limits.validate(); err != nil {
		return nil, err
	}
	tool := map[string]any{"id": input.ToolID, "revision": input.Revision}

	if input.ToolSetID != "" {
		tool["toolSetId"] = input.ToolSetID
	}

	if input.ToolSetGeneration != nil {
		tool["toolSetGeneration"] = *input.ToolSetGeneration
	}

	if input.CatalogRevision != nil {
		tool["catalogRevision"] = *input.CatalogRevision
	}

	root := map[string]any{
		"params":      input.Params,
		"cfg":         input.Cfg,
		"labels":      input.Labels,
		"annotations": input.Annotations,
		"connection": map[string]any{
			"id":        input.ConnectionID,
			"namespace": input.ConnectionNamespace,
		},
		"tool": tool,
	}

	copy, err := cloneValue(root, newBudget(context.Background(), limits), 0)
	if err != nil {
		return nil, err
	}

	snapshot := copy.(map[string]any)

	// Missing root maps are convenient empty scopes; nested nil collections
	// retain their JSON null meaning instead of becoming empty objects/arrays.
	for _, name := range []string{"params", "cfg", "labels", "annotations"} {
		if snapshot[name] == nil {
			snapshot[name] = map[string]any{}
		}
	}

	return &Context{root: snapshot}, nil
}

// cloneValue detaches a JSON tree while bounding its depth, nodes, and scalar
// storage. Numeric Go primitives become json.Number rather than float64.
func cloneValue(value any, b *budget, depth int) (any, error) {
	if err := b.step(); err != nil {
		return nil, err
	}

	if depth > b.limit.MaxDepth {
		return nil, errors.New("render depth limit exceeded")
	}

	switch v := value.(type) {
	case map[string]any:
		if v == nil {
			return nil, b.addBytes(1)
		}
		out := make(map[string]any)
		for key, child := range v {
			if !utf8.ValidString(key) {
				return nil, errors.New("context contains invalid UTF-8")
			}
			if err := b.addBytes(len(key)); err != nil {
				return nil, err
			}
			copy, err := cloneValue(child, b, depth+1)
			if err != nil {
				return nil, err
			}
			out[key] = copy
		}
		return out, nil
	case map[string]string:
		if v == nil {
			return nil, b.addBytes(1)
		}
		out := make(map[string]any)
		for key, child := range v {
			if !utf8.ValidString(key) {
				return nil, errors.New("context contains invalid UTF-8")
			}
			if err := b.addBytes(len(key)); err != nil {
				return nil, err
			}
			copy, err := cloneValue(child, b, depth+1)
			if err != nil {
				return nil, err
			}
			out[key] = copy
		}
		return out, nil
	case []any:
		if v == nil {
			return nil, b.addBytes(1)
		}
		out := make([]any, 0)
		for _, child := range v {
			copy, err := cloneValue(child, b, depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, copy)
		}
		return out, nil
	case string:
		if !utf8.ValidString(v) {
			return nil, errors.New("context contains invalid UTF-8")
		}
		return v, b.addBytes(len(v))
	case nil, bool:
		return v, b.addBytes(1)
	case json.Number, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		if number, ok := v.(json.Number); ok && number == "" {
			return nil, errors.New("context contains an invalid JSON number")
		}
		encoded, err := json.Marshal(v)
		if err != nil {
			return nil, errors.New("context contains an invalid JSON number")
		}
		return json.Number(encoded), b.addBytes(len(encoded))
	default:
		return nil, errors.New("context contains a non-JSON value")
	}
}

// lookup resolves a strict dotted reference, with innermost section scopes
// first and the invocation root last. Encoding prefixes are virtual views, so
// both json.params.object and json.params.object.child work without eager copies.
func (c *Context) lookup(name string, scopes []any) (any, error) {
	encoding := ""
	if prefix, rest, found := strings.Cut(name, "."); found &&
		(prefix == "path" || prefix == "query" || prefix == "json") {
		encoding, name = prefix, rest
	}

	value, err := c.lookupRaw(name, scopes)
	if err != nil {
		return nil, err
	}

	switch encoding {
	case "json":
		// Encoding is performed only on the already bounded JSON snapshot.
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, errors.New("cannot encode context value as JSON")
		}
		return string(encoded), nil
	case "path", "query":
		text, err := scalarText(value)
		if err != nil {
			return nil, err
		}
		if encoding == "path" {
			return url.PathEscape(text), nil
		}
		return url.QueryEscape(text), nil
	default:
		return value, nil
	}
}

// lookupRaw distinguishes absent keys from present nulls. Once a first segment
// is found, a missing descendant cannot fall back to an unrelated outer object.
func (c *Context) lookupRaw(name string, scopes []any) (any, error) {
	if name == "." && len(scopes) > 0 {
		return scopes[0], nil
	}
	if name == "." {
		return c.root, nil
	}

	parts := strings.Split(name, ".")
	for _, part := range parts {
		if part == "" {
			return nil, errors.New("invalid empty reference segment")
		}
	}

	for i := 0; i <= len(scopes); i++ {
		var scope any = c.root
		if i < len(scopes) {
			scope = scopes[i]
		}

		object, ok := scope.(map[string]any)
		if !ok {
			continue
		}

		value, exists := object[parts[0]]
		if !exists {
			continue
		}

		for _, part := range parts[1:] {
			object, ok = value.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("missing template reference %q", name)
			}
			value, exists = object[part]
			if !exists {
				return nil, fmt.Errorf("missing template reference %q", name)
			}
		}

		return value, nil
	}

	return nil, fmt.Errorf("missing template reference %q", name)
}

// scalarText defines raw interpolation without HTML escaping or Go formatting
// of objects. Arrays and objects require the JSON view or a typed directive.
func scalarText(value any) (string, error) {
	switch v := value.(type) {
	case nil:
		return "null", nil
	case string:
		return v, nil
	case bool:
		if v {
			return "true", nil
		}
		return "false", nil
	case json.Number:
		return string(v), nil
	default:
		return "", errors.New("scalar interpolation requires a string, number, boolean, or null")
	}
}
