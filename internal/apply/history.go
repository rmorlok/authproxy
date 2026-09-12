package apply

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/rmorlok/authproxy/internal/apserde"
	"github.com/rmorlok/authproxy/internal/schema/registry"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"gopkg.in/yaml.v3"
)

const LastAppliedAnnotation = "authproxy.net/last-applied-configuration"
const historyVersion = 1

// History contains desired field presence, never secret values or their hashes.
// Secret paths use RFC 6901 JSON pointers. It is private to client-side apply.
type History struct {
	Version int            `json:"version"`
	Desired map[string]any `json:"desired"`
	Secrets []string       `json:"secrets,omitempty"`
}

func plainObject(value any) (map[string]any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("cannot encode resource")
	}

	var object map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	if err := decoder.Decode(&object); err != nil {
		return nil, fmt.Errorf("invalid resource object")
	}

	return object, nil
}

func pointer(path []string) string {
	result := ""
	for _, part := range path {
		result += "/" + strings.ReplaceAll(strings.ReplaceAll(part, "~", "~0"), "/", "~1")
	}

	return result
}

func pointerParts(path string) []string {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i := range parts {
		parts[i] = strings.ReplaceAll(strings.ReplaceAll(parts[i], "~1", "/"), "~0", "~")
	}
	return parts
}

func at(value any, path []string) (any, bool) {
	if len(path) == 0 {
		return value, true
	}
	switch v := value.(type) {
	case map[string]any:
		child, ok := v[path[0]]
		if !ok {
			return nil, false
		}
		return at(child, path[1:])
	case []any:
		i, err := strconv.Atoi(path[0])
		if err != nil || i < 0 || i >= len(v) {
			return nil, false
		}
		return at(v[i], path[1:])
	}
	return nil, false
}

// Drop the entire enclosing array when a secret occurs inside one: array
// elements have no stable ownership identity and must remain atomic.
func exclude(value map[string]any, path []string) {
	if len(path) == 0 {
		return
	}
	if len(path) == 1 {
		delete(value, path[0])
		return
	}
	switch child := value[path[0]].(type) {
	case map[string]any:
		exclude(child, path[1:])
	case []any:
		delete(value, path[0])
	}
}

func secretPaths(kind meta.Kind, resource any) [][]string {
	paths := apserde.SecretPaths(resource)
	switch kind {
	case "Actor":
		paths = append(paths, []string{"spec", "signingKey"})
	case "Key":
		paths = append(paths, []string{"spec", "keyData"})
	}
	return paths
}

func newHistory(doc Document) (*History, error) {
	object, err := plainObject(doc.Object)
	if err != nil {
		return nil, err
	}
	h := &History{Version: historyVersion, Desired: object}
	for _, path := range secretPaths(doc.Kind, doc.Resource) {
		if _, ok := at(object, path); ok {
			h.Secrets = append(h.Secrets, pointer(path))
		}
		exclude(object, path)
	}
	m, _ := object["metadata"].(map[string]any)
	if ann, ok := m["annotations"].(map[string]any); ok {
		delete(ann, LastAppliedAnnotation)
	}
	h.Secrets = uniqueSorted(h.Secrets)
	return h, nil
}

func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, v := range values {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	sort.Strings(result)
	return result
}

func readHistory(raw string, kind meta.Kind) (*History, error) {
	if len(raw) > meta.AnnotationsTotalMaxSize {
		return nil, fmt.Errorf("last-applied history exceeds annotation limit")
	}
	var node yaml.Node
	if yaml.Unmarshal([]byte(raw), &node) != nil || checkNode(&node) != nil {
		return nil, fmt.Errorf("invalid last-applied history")
	}
	var h History
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if decoder.Decode(&h) != nil || h.Version != historyVersion || h.Desired == nil {
		return nil, fmt.Errorf("invalid or unsupported last-applied history")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, fmt.Errorf("invalid last-applied history")
	}
	data, _ := json.Marshal(h.Desired)
	resource, err := registry.NewResourceScheme().DecodeJSON(data)
	if err != nil {
		return nil, fmt.Errorf("invalid last-applied resource")
	}
	m, k, err := resourceMetadata(resource)
	if err != nil || k != kind || normalizeIdentity(string(kind), &m, "") != nil {
		return nil, fmt.Errorf("invalid last-applied identity")
	}
	// A history annotation is untrusted input. Never echo its payload on errors.
	if _, ok := h.Desired["status"]; ok {
		return nil, fmt.Errorf("server-owned fields in last-applied history")
	}
	clean, err := newHistory(Document{Kind: kind, Object: h.Desired, Resource: resource})
	if err != nil {
		return nil, err
	}
	a, _ := json.Marshal(clean.Desired)
	b, _ := json.Marshal(h.Desired)
	if !bytes.Equal(a, b) {
		return nil, fmt.Errorf("last-applied history contains excluded fields")
	}
	for _, p := range h.Secrets {
		if !strings.HasPrefix(p, "/spec/") || pointer(pointerParts(p)) != p {
			return nil, fmt.Errorf("invalid secret presence history")
		}
	}
	return &h, nil
}
