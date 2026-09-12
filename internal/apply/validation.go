package apply

import (
	"fmt"
	"sort"
	"sync"

	"github.com/rmorlok/authproxy/internal/schema"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v5"
)

type Validation string

const (
	ValidationStrict Validation = "strict"
	ValidationWarn   Validation = "warn"
	ValidationIgnore Validation = "ignore"
)

func (v Validation) valid() bool {
	return v == "" || v == ValidationStrict || v == ValidationWarn || v == ValidationIgnore
}

var fieldSchemaMu sync.Mutex

// Unknown-field modes affect field discovery only. Strict typed decoding and
// all identity, duplicate-key, placeholder and lifecycle checks still run.
func (l *inputLoader) filterUnknown(source string, object map[string]any, kind meta.Kind) error {
	if l.options.Validation == "" || l.options.Validation == ValidationStrict {
		return nil
	}
	descriptor, err := resourceType(kind)
	if err != nil {
		return err
	}
	fieldSchemaMu.Lock()
	contract, err := schema.CompileSchema(descriptor.SchemaRef)
	fieldSchemaMu.Unlock()
	if err != nil {
		return fmt.Errorf("cannot load resource field schema")
	}
	filtered, removed := pruneUnknown(contract, object, 0)
	for key := range object {
		delete(object, key)
	}
	for key, value := range filtered.(map[string]any) {
		object[key] = value
	}
	l.warnUnknown(source, removed)

	return nil
}

func (l *inputLoader) warnUnknown(source string, removed int) {
	if removed > 0 && l.options.Validation == ValidationWarn && l.options.Warn != nil {
		// Field names can themselves contain sensitive data in malformed input.
		l.options.Warn(fmt.Sprintf("%s: ignored %d unknown field(s)", source, removed))
	}
}

// pruneUnknown follows the embedded contract, including polymorphic unions.
// Each alternative is tried on a copy. Only a branch whose scalar constraints
// match is eligible; ambiguous/invalid unions are left for strict decoding.
func pruneUnknown(s *jsonschema.Schema, value any, depth int) (any, int) {
	if s == nil || depth > 128 {
		return value, 0
	}
	result := value
	removed := 0
	apply := func(child *jsonschema.Schema) { v, n := pruneUnknown(child, result, depth+1); result = v; removed += n }
	if s.Ref != nil {
		apply(s.Ref)
	}
	for _, child := range s.AllOf {
		apply(child)
	}
	for _, union := range [][]*jsonschema.Schema{s.AnyOf, s.OneOf} {
		bestCount := -1
		var best any
		ambiguous := false
		for _, child := range union {
			if !compatible(child, result, depth+1) {
				continue
			}
			candidate, count := pruneUnknown(child, result, depth+1)
			if bestCount < 0 {
				best = candidate
				bestCount = count
				continue
			}
			if subsetJSON(best, candidate) {
				best = candidate
				bestCount = count
			} else if !subsetJSON(candidate, best) {
				ambiguous = true
			}
		}
		// Never turn competing provider configurations into an arbitrary valid one.
		// Leave incomparable alternatives intact so strict decoding rejects them.
		if bestCount >= 0 && !ambiguous {
			result = best
			removed += bestCount
		}
	}

	switch v := result.(type) {
	case map[string]any:
		output := asMap(v)
		for _, key := range keys(v) {
			if child, ok := s.Properties[key]; ok {
				next, n := pruneUnknown(child, v[key], depth+1)
				output[key] = next
				removed += n
				continue
			}
			matched := false
			for pattern, child := range s.PatternProperties {
				if pattern.MatchString(key) {
					next, n := pruneUnknown(child, output[key], depth+1)
					output[key] = next
					removed += n
					matched = true
				}
			}
			if matched {
				continue
			}
			switch extra := s.AdditionalProperties.(type) {
			case bool:
				if !extra {
					delete(output, key)
					removed++
				}
			case *jsonschema.Schema:
				next, n := pruneUnknown(extra, v[key], depth+1)
				output[key] = next
				removed += n
			}
		}
		// Resource envelopes close properties across allOf using unevaluatedProperties.
		if s.UnevaluatedProperties != nil && s.UnevaluatedProperties.Always != nil && !*s.UnevaluatedProperties.Always {
			allowed := map[string]bool{}
			collectProperties(s, allowed, 0)
			for key := range output {
				if !allowed[key] {
					delete(output, key)
					removed++
				}
			}
		}
		result = output
	case []any:
		output := append([]any{}, v...)
		child := s.Items2020
		if c, ok := s.Items.(*jsonschema.Schema); ok {
			child = c
		}
		for i := range output {
			itemSchema := child
			if i < len(s.PrefixItems) {
				itemSchema = s.PrefixItems[i]
			}
			next, n := pruneUnknown(itemSchema, output[i], depth+1)
			output[i] = next
			removed += n
		}
		result = output
	}
	return result, removed
}
func collectProperties(s *jsonschema.Schema, result map[string]bool, depth int) {
	if s == nil || depth > 128 {
		return
	}
	for key := range s.Properties {
		result[key] = true
	}
	collectProperties(s.Ref, result, depth+1)
	for _, children := range [][]*jsonschema.Schema{s.AllOf, s.AnyOf, s.OneOf} {
		for _, child := range children {
			collectProperties(child, result, depth+1)
		}
	}
}
func compatible(s *jsonschema.Schema, value any, depth int) bool {
	if s == nil || depth > 128 {
		return true
	}
	if s.Always != nil && !*s.Always {
		return false
	}
	for _, union := range [][]*jsonschema.Schema{s.AnyOf, s.OneOf} {
		if len(union) == 0 {
			continue
		}
		matched := false
		for _, child := range union {
			if compatible(child, value, depth+1) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if s.Ref != nil && !compatible(s.Ref, value, depth+1) {
		return false
	}
	// Scalar validation includes discriminator enums/consts and prevents an
	// invalid enum from selecting an unrelated union variant.
	switch value.(type) {
	case map[string]any, []any:
	default:
		return s.Validate(value) == nil
	}
	if len(s.Types) > 0 {
		expected := "object"
		if _, ok := value.([]any); ok {
			expected = "array"
		}
		found := false
		for _, t := range s.Types {
			if t == expected {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	for _, child := range s.AllOf {
		if !compatible(child, value, depth+1) {
			return false
		}
	}
	if m, ok := value.(map[string]any); ok {
		names := make([]string, 0, len(s.Properties))
		for name := range s.Properties {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if v, exists := m[name]; exists && !compatible(s.Properties[name], v, depth+1) {
				return false
			}
		}
		// Presence discriminates provider unions such as keyData.value vs base64.
		for _, key := range s.Required {
			if _, ok := m[key]; !ok {
				return false
			}
		}
	}
	return true
}

func subsetJSON(a, b any) bool {
	switch x := a.(type) {
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok {
			return false
		}
		for k, v := range x {
			w, present := y[k]
			if !present || !subsetJSON(v, w) {
				return false
			}
		}
		return true
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !subsetJSON(x[i], y[i]) {
				return false
			}
		}
		return true
	default:
		return equalJSON(a, b)
	}
}
