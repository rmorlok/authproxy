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
	// Params is the caller-validated tool input, exposed as params. Nested values
	// must be JSON primitives, map[string]any, map[string]string, or []any; typed
	// slices such as []string and arbitrary Go objects are rejected. NewContext
	// copies these caller-owned values, including nested maps and slices.
	Params map[string]any
	// Cfg is the captured connection configuration, exposed as cfg. It follows
	// the same type and copy rules as Params and must not contain credentials.
	Cfg map[string]any
	// Labels is the captured effective label set, copied into labels.
	Labels map[string]string
	// Annotations is the captured connection annotation set, copied into annotations.
	Annotations map[string]string
	// ConnectionID is the connection identity exposed as connection.id. An empty
	// value is omitted so templates that require it fail strict lookup.
	ConnectionID string
	// ConnectionNamespace is the owning namespace exposed as connection.namespace;
	// an empty value is omitted from the snapshot.
	ConnectionNamespace string
	// ToolID identifies the invoked tool as tool.id; an empty value is omitted.
	ToolID string
	// Revision identifies the captured executable tool revision, exposed as
	// tool.revision. Zero is omitted because revisions start at one. This is
	// distinct from the source catalog's revision below.
	Revision uint64
	// ToolSetID identifies the source toolset as tool.toolSetId. Empty means the
	// tool has no toolset provenance, so the field is omitted.
	ToolSetID string
	// ToolSetGeneration is the published source generation exposed as
	// tool.toolSetGeneration. Nil omits it; a non-nil value is copied by value.
	ToolSetGeneration *uint64
	// CatalogRevision is the connection-specific source catalog revision exposed
	// as tool.catalogRevision. Nil means no catalog provenance and omits it; a
	// non-nil value is copied by value.
	CatalogRevision *uint64
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
	connection := map[string]any{}
	if input.ConnectionID != "" {
		connection["id"] = input.ConnectionID
	}
	if input.ConnectionNamespace != "" {
		connection["namespace"] = input.ConnectionNamespace
	}

	tool := map[string]any{}
	if input.ToolID != "" {
		tool["id"] = input.ToolID
	}
	if input.Revision != 0 {
		tool["revision"] = input.Revision
	}

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
		"connection":  connection,
		"tool":        tool,
	}

	b := newBudget(context.Background(), limits)
	if err := b.step(); err != nil {
		return nil, err
	}
	snapshot := make(map[string]any, len(root))
	for _, name := range []string{"params", "cfg", "labels", "annotations", "connection", "tool"} {
		if err := b.addBytes(len(name)); err != nil {
			return nil, err
		}
		copy, err := cloneValue(root[name], b, 1)
		if err != nil {
			// Only these fixed root names are included: arbitrary nested keys and
			// values may contain sensitive connection configuration.
			return nil, fmt.Errorf("context field %s: %w", name, err)
		}
		snapshot[name] = copy
	}

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

// encodingReference recognizes virtual encoding views only when they explicitly
// name an invocation root. Section-relative fields such as query.text remain
// ordinary data references, while json.params and json.params.child are encoded.
func encodingReference(name string) (encoding, reference string) {
	prefix, rest, found := strings.Cut(name, ".")
	if !found || (prefix != "path" && prefix != "query" && prefix != "json") {
		return "", name
	}

	root, _, _ := strings.Cut(rest, ".")
	switch root {
	case "params", "cfg", "labels", "annotations", "connection", "tool":
		return prefix, rest
	default:
		return "", name
	}
}

// lookup resolves ordinary references from the innermost section scope first.
// Encoding views are anchored at the invocation root, so a section cannot
// replace cfg or params inside an explicitly encoded reference.
func (c *Context) lookup(name string, scopes []any) (any, error) {
	encoding, name := encodingReference(name)
	if encoding != "" {
		scopes = nil
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
			if text == "" || text == "." || text == ".." {
				return nil, errors.New("path encoding requires a nonempty, non-dot path segment")
			}
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
