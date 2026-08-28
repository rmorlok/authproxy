package manifest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/util"
	"gopkg.in/yaml.v3"
)

// GVK is the lookup key for a registered manifest type.
type GVK struct {
	APIVersion meta.APIVersion
	Kind       meta.Kind
}

func (g GVK) String() string { return fmt.Sprintf("%s, Kind=%s", g.APIVersion, g.Kind) }

// Factory creates a fresh pointer to a registered manifest contract.
type Factory func() any

// Scheme dispatches strict JSON and YAML manifests by apiVersion and kind.
// It is safe for concurrent registration and decoding.
type Scheme struct {
	mu        sync.RWMutex
	factories map[GVK]Factory
	versions  map[meta.APIVersion]struct{}
}

func NewScheme() *Scheme {
	return &Scheme{
		factories: make(map[GVK]Factory),
		versions:  make(map[meta.APIVersion]struct{}),
	}
}

func (s *Scheme) Register(gvk GVK, factory Factory) error {
	if s == nil {
		return fmt.Errorf("manifest scheme is nil")
	}
	if err := meta.ValidateTypeMeta(meta.TypeMeta{APIVersion: gvk.APIVersion, Kind: gvk.Kind}, "", "", nil); err != nil {
		return fmt.Errorf("invalid registration %s: %w", gvk, err)
	}
	if factory == nil {
		return fmt.Errorf("register %s: factory is required", gvk)
	}
	sample := factory()
	if err := validateFactoryResult(sample); err != nil {
		return fmt.Errorf("register %s: %w", gvk, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.factories[gvk]; exists {
		return fmt.Errorf("register %s: already registered", gvk)
	}
	s.factories[gvk] = factory
	s.versions[gvk.APIVersion] = struct{}{}
	return nil
}

func RegisterType[T any](scheme *Scheme, gvk GVK) error {
	return scheme.Register(gvk, func() any { return new(T) })
}

func MustRegisterType[T any](scheme *Scheme, gvk GVK) {
	if err := RegisterType[T](scheme, gvk); err != nil {
		panic(err)
	}
}

func (s *Scheme) RegisteredGVKs() []GVK {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	result := make([]GVK, 0, len(s.factories))
	for gvk := range s.factories {
		result = append(result, gvk)
	}
	s.mu.RUnlock()
	sort.Slice(result, func(i, j int) bool {
		if result[i].APIVersion != result[j].APIVersion {
			return result[i].APIVersion < result[j].APIVersion
		}
		return result[i].Kind < result[j].Kind
	})
	return result
}

func (s *Scheme) DecodeJSON(data []byte) (any, error) {
	typeMeta, err := decodeJSONTypeMeta(data)
	if err != nil {
		return nil, err
	}
	factory, err := s.resolve(typeMeta)
	if err != nil {
		return nil, err
	}
	result := factory()
	if err := validateFactoryResult(result); err != nil {
		return nil, fmt.Errorf("decode %s: registered factory %w", GVK{typeMeta.APIVersion, typeMeta.Kind}, err)
	}
	if err := util.DecodeJSONStrict(data, result); err != nil {
		return nil, fmt.Errorf("decode %s JSON: %w", GVK{typeMeta.APIVersion, typeMeta.Kind}, err)
	}
	return result, nil
}

// DecodeYAML decodes exactly one non-empty YAML document.
func (s *Scheme) DecodeYAML(data []byte) (any, error) {
	results, err := s.DecodeYAMLDocuments(data)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("decode YAML: expected one document, got none")
	}
	if len(results) != 1 {
		return nil, fmt.Errorf("decode YAML: expected one document, got %d", len(results))
	}
	return results[0], nil
}

// DecodeYAMLDocuments decodes a YAML stream, skips empty documents, and
// dispatches every remaining document independently by GVK.
func (s *Scheme) DecodeYAMLDocuments(data []byte) ([]any, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var results []any
	for document := 1; ; document++ {
		var node yaml.Node
		if err := decoder.Decode(&node); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("decode YAML document %d: %w", document, err)
		}
		if yamlDocumentEmpty(&node) {
			continue
		}
		payload, err := yaml.Marshal(&node)
		if err != nil {
			return nil, fmt.Errorf("encode YAML document %d: %w", document, err)
		}
		result, err := s.decodeYAMLDocument(payload)
		if err != nil {
			return nil, fmt.Errorf("decode YAML document %d: %w", document, err)
		}
		results = append(results, result)
	}
	return results, nil
}

func (s *Scheme) decodeYAMLDocument(data []byte) (any, error) {
	var typeMeta meta.TypeMeta
	if err := yaml.Unmarshal(data, &typeMeta); err != nil {
		return nil, fmt.Errorf("decode type metadata: %w", err)
	}
	if err := validatePresentTypeMeta(typeMeta); err != nil {
		return nil, err
	}
	factory, err := s.resolve(typeMeta)
	if err != nil {
		return nil, err
	}
	result := factory()
	if err := validateFactoryResult(result); err != nil {
		return nil, fmt.Errorf("decode %s: registered factory %w", GVK{typeMeta.APIVersion, typeMeta.Kind}, err)
	}
	if err := util.DecodeYAMLStrict(data, result); err != nil {
		return nil, fmt.Errorf("decode %s YAML: %w", GVK{typeMeta.APIVersion, typeMeta.Kind}, err)
	}
	return result, nil
}

func decodeJSONTypeMeta(data []byte) (meta.TypeMeta, error) {
	var raw map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&raw); err != nil {
		return meta.TypeMeta{}, fmt.Errorf("decode type metadata: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return meta.TypeMeta{}, fmt.Errorf("decode type metadata: unexpected additional JSON value")
		}
		return meta.TypeMeta{}, fmt.Errorf("decode type metadata: %w", err)
	}

	var result meta.TypeMeta
	if value, exists := raw["apiVersion"]; exists {
		if err := json.Unmarshal(value, &result.APIVersion); err != nil {
			return meta.TypeMeta{}, (&common.ValidationContext{Path: "$"}).NewErrorfForField("apiVersion", "must be a string: %v", err)
		}
	}
	if value, exists := raw["kind"]; exists {
		if err := json.Unmarshal(value, &result.Kind); err != nil {
			return meta.TypeMeta{}, (&common.ValidationContext{Path: "$"}).NewErrorfForField("kind", "must be a string: %v", err)
		}
	}
	if err := validatePresentTypeMeta(result); err != nil {
		return meta.TypeMeta{}, err
	}
	return result, nil
}

func validatePresentTypeMeta(value meta.TypeMeta) error {
	return meta.ValidateTypeMeta(value, "", "", &common.ValidationContext{Path: "$"})
}

func (s *Scheme) resolve(value meta.TypeMeta) (Factory, error) {
	if s == nil {
		return nil, fmt.Errorf("manifest scheme is nil")
	}
	gvk := GVK{APIVersion: value.APIVersion, Kind: value.Kind}
	s.mu.RLock()
	factory, exists := s.factories[gvk]
	_, versionExists := s.versions[value.APIVersion]
	s.mu.RUnlock()
	if exists {
		return factory, nil
	}
	vc := &common.ValidationContext{Path: "$"}
	if !versionExists {
		return nil, vc.NewErrorfForField("apiVersion", "unsupported value %q", value.APIVersion)
	}
	return nil, vc.NewErrorfForField("kind", "unsupported value %q for apiVersion %q", value.Kind, value.APIVersion)
}

func yamlDocumentEmpty(node *yaml.Node) bool {
	if node == nil || len(node.Content) == 0 {
		return true
	}
	value := node.Content[0]
	return value.Kind == yaml.ScalarNode && value.Tag == "!!null" && strings.TrimSpace(value.Value) == ""
}

func validateFactoryResult(value any) error {
	if value == nil {
		return fmt.Errorf("returned nil")
	}
	valueType := reflect.TypeOf(value)
	if valueType.Kind() != reflect.Pointer {
		return fmt.Errorf("must return a pointer, got %s", valueType)
	}
	if reflect.ValueOf(value).IsNil() {
		return fmt.Errorf("returned a nil %s", valueType)
	}
	return nil
}
