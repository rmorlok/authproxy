package apply

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/rmorlok/authproxy/internal/apserde"
	apiv1alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	ns "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/util"
	"gopkg.in/yaml.v3"
)

// Document retains the normalized input, including explicit null and empty
// values. Object may contain secrets: use RedactedObject for display, never log
// Object or Resource. Source identifies the file, document and list item.
type Document struct {
	Source   string
	Kind     meta.Kind
	Metadata meta.ObjectMeta
	Object   map[string]any
	Resource any
}

// RedactedObject masks schema-declared secrets while preserving input field
// presence. It never returns the underlying Object map.
func (d Document) RedactedObject() (any, error) {
	safe, _, err := apserde.SanitizeJSONForAPI(context.Background(), d.Resource)
	if err != nil {
		return nil, fmt.Errorf("%s: cannot redact resource", d.Source)
	}
	return project(d.Object, safe), nil
}

func project(input, safe any) any {
	if input == nil {
		return nil
	}
	switch value := input.(type) {
	case map[string]any:
		result := map[string]any{}
		sanitized, _ := safe.(map[string]any)
		for k, v := range value {
			result[k] = project(v, sanitized[k])
		}
		return result
	case []any:
		result := make([]any, len(value))
		sanitized, _ := safe.([]any)
		for i, v := range value {
			var s any
			if i < len(sanitized) {
				s = sanitized[i]
			}
			result[i] = project(v, s)
		}
		return result
	default:
		if _, ok := safe.(json.Number); ok {
			return input
		}
		if safe != nil {
			return safe
		}
		// A field omitted by the typed serializer has no printable value. Retain
		// explicit null/zero/empty scalars, but fail closed for other values.
		if input == "" ||
			input == false ||
			input == 0 ||
			input == int64(0) ||
			input == uint64(0) ||
			input == float64(0) {
			return input
		}
		return nil
	}
}

type decoder interface{ DecodeYAML([]byte) (any, error) }

func (l *inputLoader) decode(source string, data []byte) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	for index := 1; ; index++ {
		if err := l.ctx.Err(); err != nil {
			return err
		}

		var node yaml.Node
		err := dec.Decode(&node)
		if err == io.EOF {
			return nil
		}

		location := fmt.Sprintf("%s: document %d", source, index)
		// YAML/type conversion errors can embed scalar values, including secrets.
		if err != nil {
			return fmt.Errorf("%s: malformed YAML or JSON", location)
		}

		if util.YamlDocumentEmpty(&node) {
			continue
		}

		if err := checkNode(&node); err != nil {
			return fmt.Errorf("%s: %w", location, err)
		}

		var object map[string]any
		if err := node.Decode(&object); err != nil || object == nil {
			return fmt.Errorf("%s: expected a resource object", location)
		}

		if err := l.object(location, object, ""); err != nil {
			return err
		}
	}
}

// Reject duplicate/non-string keys even in arbitrary maps and JSON documents.
// Aliases and merge keys are deliberately rejected to keep presence unambiguous.
func checkNode(n *yaml.Node) error {
	if n.Kind == yaml.AliasNode {
		return fmt.Errorf("line %d: YAML aliases are unsupported", n.Line)
	}

	if n.Kind == yaml.MappingNode {
		seen := map[string]bool{}
		for i := 0; i < len(n.Content); i += 2 {
			k := n.Content[i]
			if k.Tag != "!!str" {
				return fmt.Errorf("line %d: mapping keys must be strings; YAML merge keys are unsupported", k.Line)
			}
			if seen[k.Value] {
				return fmt.Errorf("line %d: duplicate mapping key", k.Line)
			}
			seen[k.Value] = true
		}
	}

	for _, child := range n.Content {
		if err := checkNode(child); err != nil {
			return err
		}
	}

	return nil
}

func (l *inputLoader) object(
	source string,
	object map[string]any,
	expectedKind string,
) error {
	fail := func(message string) error { return fmt.Errorf("%s: %s", source, message) }
	version, _ := object["apiVersion"].(string)
	kind, _ := object["kind"].(string)

	if version != string(meta.APIVersionV1Alpha1) {
		return fail("unsupported or missing apiVersion (expected authproxy.net/v1alpha1)")
	}

	if expectedKind != "" &&
		expectedKind != "*" &&
		kind != expectedKind {
		return fail("list item kind does not match its list")
	}

	if strings.HasSuffix(kind, "List") || kind == "List" {
		if expectedKind != "" {
			return fail("nested lists are unsupported")
		}

		base := strings.TrimSuffix(kind, "List")
		if base != "" && !supportedKind(base) {
			return fail("unsupported list kind")
		}

		for field := range object {
			if field != "apiVersion" &&
				field != "kind" &&
				field != "metadata" &&
				field != "items" {
				return fail("unknown list field")
			}
		}

		if value, ok := object["metadata"]; ok {
			payload, _ := yaml.Marshal(value)

			var m apiv1alpha1.ListMeta

			if err := util.DecodeYAMLStrict(payload, &m); err != nil {
				return fail("invalid list metadata")
			}

			if m.Continue != "" ||
				(m.RemainingItemCount != nil && *m.RemainingItemCount > 0) {
				return fail("incomplete paginated list; supply all resources")
			}
		}

		items, ok := object["items"].([]any)
		if !ok {
			return fail("list items must be an array")
		}

		if base == "" {
			base = "*"
		}

		for i, item := range items {
			child, ok := item.(map[string]any)
			if !ok {
				return fail(fmt.Sprintf("item %d: expected resource object", i+1))
			}

			if err := l.object(fmt.Sprintf("%s: item %d", source, i+1), child, base); err != nil {
				return err
			}
		}
		return nil
	}

	if !supportedKind(kind) {
		return fail("unsupported or missing resource kind")
	}

	if _, ok := object["status"]; ok {
		return fail("status is server-owned")
	}

	metadata, ok := object["metadata"].(map[string]any)
	if !ok {
		return fail("metadata must be an object")
	}

	for _, field := range []string{"createdAt", "updatedAt"} {
		if _, ok := metadata[field]; ok {
			return fail("metadata." + field + " is server-owned")
		}
	}

	if _, ok := metadata["generation"]; ok && kind != "Connector" {
		return fail("metadata.generation is only supported for Connector targets")
	}

	if value, exists := object["spec"]; exists {
		if _, ok := value.(map[string]any); !ok {
			return fail("spec must be an object")
		}
	}

	if kind == "Connection" {
		if spec, ok := object["spec"].(map[string]any); ok && len(spec) > 0 {
			return fail("Connection apply supports existing mutable metadata only")
		}
	}

	for _, field := range []string{"id", "name", "namespace"} {
		if value, exists := metadata[field]; exists {
			if str, ok := value.(string); !ok || str == "" {
				return fail("metadata." + field + " must be a nonempty string when supplied")
			}
		}
	}

	var m meta.ObjectMeta
	payload, err := yaml.Marshal(metadata)
	if err != nil {
		return fail("invalid metadata")
	}

	if err := util.DecodeYAMLStrict(payload, &m); err != nil {
		return fail("invalid metadata fields or types")
	}

	if _, supplied := metadata["generation"]; supplied && m.Generation == 0 {
		return fail("metadata.generation must be positive when supplied")
	}

	if err := normalizeIdentity(kind, &m, l.options.Namespace); err != nil {
		return fail(err.Error())
	}

	if m.Namespace != "" {
		metadata["namespace"] = m.Namespace
	}

	if m.Name != "" {
		metadata["name"] = string(m.Name)
	}

	if err := meta.ValidateUserLabels(m.Labels); err != nil {
		return fail("invalid metadata.labels")
	}

	if err := meta.ValidateAnnotations(m.Annotations); err != nil {
		return fail("invalid metadata.annotations")
	}

	payload, err = yaml.Marshal(object)
	if err != nil {
		return fail("cannot encode resource")
	}

	typed, err := l.scheme.DecodeYAML(payload)
	if err != nil {
		return fail("resource contains unknown fields or invalid field types")
	}

	if err := apserde.ValidateNoRedactedPlaceholders(typed); err != nil {
		return fail("redacted placeholders cannot be applied")
	}

	l.count++
	if !l.selector.Matches(m.Labels) {
		return nil
	}

	doc := Document{Source: source, Kind: meta.Kind(kind), Metadata: m, Object: object, Resource: typed}
	identities := []string{}
	if m.ID != "" {
		identities = append(identities, kind+"/id/"+m.ID)
	}

	if m.Name != "" && (m.Namespace != "" || kind == "Namespace") {
		identities = append(identities, kind+"/name/"+m.Namespace+"/"+string(m.Name))
	}

	for _, identity := range identities {
		if previous, exists := l.seen[identity]; exists {
			return fail("duplicate resource identity; first defined at " + previous)
		}
		l.seen[identity] = source
	}

	l.documents = append(l.documents, doc)

	return nil
}

func supportedKind(kind string) bool {
	switch kind {
	case "Namespace",
		"Actor",
		"Connector",
		"Key",
		"RateLimit",
		"Connection":
		return true
	}
	return false
}

func validateNamespace(value string) error { return ns.ValidatePath(value) }

func normalizeIdentity(kind string, m *meta.ObjectMeta, fallback string) error {
	if kind == "Namespace" && m.ID != "" {
		canonical, err := ns.NewResourceMetadata(m.ID)
		if err != nil {
			return fmt.Errorf("invalid Namespace metadata.id")
		}

		if (m.Name != "" && m.Name != canonical.Name) ||
			(m.Namespace != "" && m.Namespace != canonical.Namespace) {
			return fmt.Errorf("Namespace identity fields disagree")
		}

		m.Name = canonical.Name
		m.Namespace = canonical.Namespace
	} else {
		root := kind == "Namespace" &&
			m.Name == common.ResourceName(ns.Root) &&
			m.Namespace == ""
		if m.Namespace == "" && !root {
			m.Namespace = fallback
		}
	}

	if m.Namespace != "" {
		if err := ns.ValidatePath(m.Namespace); err != nil {
			return fmt.Errorf("invalid metadata.namespace")
		}
	}

	if m.Name != "" {
		if err := m.Name.Validate(); err != nil {
			return fmt.Errorf("invalid metadata.name")
		}
	}

	if kind == "Namespace" {
		if _, err := ns.PathFromMetadata(*m); err != nil {
			return fmt.Errorf("Namespace requires id or name and parent namespace (except root)")
		}
		return nil
	}

	if m.ID == "" && (m.Name == "" || m.Namespace == "") {
		return fmt.Errorf("metadata.id or metadata.namespace and metadata.name are required; use --namespace to supply a missing namespace")
	}

	if m.ID != "" {
		descriptor, err := resourceType(meta.Kind(kind))
		if err != nil {
			return err
		}
		if err := descriptor.ValidateID(m.ID); err != nil {
			return fmt.Errorf("invalid metadata.id for %s", kind)
		}
	}

	return nil
}
