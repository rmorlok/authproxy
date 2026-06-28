package apserde

import (
	"bytes"
	"context"
	"encoding"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const (
	RedactTagName           = "apiredact"
	RedactTagSecret         = "secret"
	SecretResource          = "secrets"
	SecretReplayVerb        = "replay"
	RedactedHeader          = "X-AuthProxy-Data-Redacted"
	redactionRune           = "*"
	secretReplayKey         = contextKey("secretReplay")
	formatJSON       format = iota
	formatYAML
)

type contextKey string

type format int

// Report describes what happened during API sanitization.
type Report struct {
	Redacted bool
}

// WithSecretReplay records whether the current request may replay secret values.
func WithSecretReplay(ctx context.Context, allowed bool) context.Context {
	return context.WithValue(ctx, secretReplayKey, allowed)
}

// SecretReplayAllowed returns true when API serializers should emit original secret values.
func SecretReplayAllowed(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	allowed, _ := ctx.Value(secretReplayKey).(bool)
	return allowed
}

// SanitizeJSONForAPI converts v into JSON-ready data with API redaction applied.
func SanitizeJSONForAPI(ctx context.Context, v any) (any, Report, error) {
	return sanitizeForAPI(ctx, v, formatJSON)
}

// SanitizeYAMLForAPI converts v into YAML-ready data with API redaction applied.
func SanitizeYAMLForAPI(ctx context.Context, v any) (any, Report, error) {
	return sanitizeForAPI(ctx, v, formatYAML)
}

// MarshalJSONForAPI renders v as JSON with API redaction applied and HTML escaping disabled.
func MarshalJSONForAPI(ctx context.Context, v any) ([]byte, Report, error) {
	sanitized, report, err := SanitizeJSONForAPI(ctx, v)
	if err != nil {
		return nil, report, err
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(sanitized); err != nil {
		return nil, report, err
	}
	return buf.Bytes(), report, nil
}

// MarshalYAMLForAPI renders v as YAML with API redaction applied.
func MarshalYAMLForAPI(ctx context.Context, v any) ([]byte, Report, error) {
	sanitized, report, err := SanitizeYAMLForAPI(ctx, v)
	if err != nil {
		return nil, report, err
	}
	out, err := yaml.Marshal(sanitized)
	return out, report, err
}

func sanitizeForAPI(ctx context.Context, v any, fmtType format) (any, Report, error) {
	if SecretReplayAllowed(ctx) || !valueHasRedactionTag(reflect.ValueOf(v), map[visit]bool{}) {
		plain, err := toPlain(fmtType, v)
		return plain, Report{}, err
	}

	report := Report{}
	sanitized, err := sanitizeValue(fmtType, reflect.ValueOf(v), &report)
	return sanitized, report, err
}

func sanitizeValue(fmtType format, v reflect.Value, report *Report) (any, error) {
	if !v.IsValid() {
		return nil, nil
	}

	v = unwrapInterface(v)
	if !v.IsValid() {
		return nil, nil
	}
	if isNil(v) {
		return nil, nil
	}

	if !valueHasRedactionTag(v, map[visit]bool{}) {
		return toPlain(fmtType, v.Interface())
	}

	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil, nil
		}
		v = unwrapInterface(v.Elem())
	}

	if inner, ok := innerValue(v); ok && valueHasRedactionTag(inner, map[visit]bool{}) {
		return sanitizeValue(fmtType, inner, report)
	}

	switch v.Kind() {
	case reflect.Struct:
		return sanitizeStruct(fmtType, v, report)
	case reflect.Slice, reflect.Array:
		items := make([]any, 0, v.Len())
		for i := 0; i < v.Len(); i++ {
			item, err := sanitizeValue(fmtType, v.Index(i), report)
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		return items, nil
	case reflect.Map:
		result := make(map[string]any, v.Len())
		iter := v.MapRange()
		for iter.Next() {
			key, err := mapKeyToString(iter.Key())
			if err != nil {
				return nil, err
			}
			value, err := sanitizeValue(fmtType, iter.Value(), report)
			if err != nil {
				return nil, err
			}
			result[key] = value
		}
		return result, nil
	default:
		return toPlain(fmtType, v.Interface())
	}
}

func sanitizeStruct(fmtType format, v reflect.Value, report *Report) (map[string]any, error) {
	result := map[string]any{}
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" && !field.Anonymous {
			continue
		}

		name, opts, ok := fieldName(fmtType, field)
		if !ok {
			continue
		}

		fv := v.Field(i)
		if opts.Contains("omitempty") && isEmptyValue(fv) {
			continue
		}

		if isSecretField(field) {
			redacted, didRedact, err := redactValue(fmtType, fv)
			if err != nil {
				return nil, err
			}
			if didRedact {
				report.Redacted = true
			}
			result[name] = redacted
			continue
		}

		sanitized, err := sanitizeValue(fmtType, fv, report)
		if err != nil {
			return nil, err
		}
		result[name] = sanitized
	}

	return result, nil
}

func redactValue(fmtType format, v reflect.Value) (any, bool, error) {
	if !v.IsValid() || isNil(v) {
		return nil, false, nil
	}

	plain, err := toPlain(fmtType, v.Interface())
	if err != nil {
		return nil, false, err
	}
	return maskPlain(plain)
}

func maskPlain(v any) (any, bool, error) {
	switch typed := v.(type) {
	case nil:
		return nil, false, nil
	case string:
		if typed == "" {
			return typed, false, nil
		}
		return strings.Repeat(redactionRune, utf8.RuneCountInString(typed)), true, nil
	case []any:
		redacted := false
		out := make([]any, len(typed))
		for i, item := range typed {
			masked, didMask, err := maskPlain(item)
			if err != nil {
				return nil, false, err
			}
			out[i] = masked
			redacted = redacted || didMask
		}
		return out, redacted, nil
	case map[string]any:
		redacted := false
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			masked, didMask, err := maskPlain(item)
			if err != nil {
				return nil, false, err
			}
			out[key] = masked
			redacted = redacted || didMask
		}
		return out, redacted, nil
	default:
		text := fmt.Sprint(typed)
		if text == "" {
			return typed, false, nil
		}
		return strings.Repeat(redactionRune, utf8.RuneCountInString(text)), true, nil
	}
}

// ValidateNoRedactedPlaceholders rejects mask-only values supplied for annotated secret fields.
func ValidateNoRedactedPlaceholders(v any) error {
	paths := []string{}
	if err := collectRedactedPlaceholders(reflect.ValueOf(v), "$", &paths); err != nil {
		return err
	}
	if len(paths) > 0 {
		return fmt.Errorf("redacted placeholder values are not accepted for secret fields: %s", strings.Join(paths, ", "))
	}
	return nil
}

func collectRedactedPlaceholders(v reflect.Value, path string, paths *[]string) error {
	if !v.IsValid() || isNil(v) {
		return nil
	}

	v = unwrapInterface(v)
	for v.IsValid() && v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = unwrapInterface(v.Elem())
	}
	if !v.IsValid() {
		return nil
	}

	if inner, ok := innerValue(v); ok {
		return collectRedactedPlaceholders(inner, path, paths)
	}

	if v.Kind() != reflect.Struct {
		return nil
	}

	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" && !field.Anonymous {
			continue
		}
		name, _, ok := fieldName(formatJSON, field)
		if !ok {
			continue
		}
		fv := v.Field(i)
		fieldPath := path + "." + name
		if isSecretField(field) {
			plain, err := toPlain(formatJSON, fv.Interface())
			if err != nil {
				return err
			}
			if containsMaskOnlyString(plain) {
				*paths = append(*paths, fieldPath)
			}
			continue
		}
		if err := collectRedactedPlaceholders(fv, fieldPath, paths); err != nil {
			return err
		}
	}

	return nil
}

func containsMaskOnlyString(v any) bool {
	switch typed := v.(type) {
	case string:
		if typed == "" {
			return false
		}
		for _, r := range typed {
			if r != '*' {
				return false
			}
		}
		return true
	case []any:
		return slices.ContainsFunc(typed, containsMaskOnlyString)
	case map[string]any:
		for _, item := range typed {
			if containsMaskOnlyString(item) {
				return true
			}
		}
	}
	return false
}

type tagOptions []string

func (o tagOptions) Contains(option string) bool {
	return slices.Contains(o, option)
}

func fieldName(fmtType format, field reflect.StructField) (string, tagOptions, bool) {
	tagName := "json"
	if fmtType == formatYAML {
		tagName = "yaml"
	}

	tag := field.Tag.Get(tagName)
	if tag == "-" {
		return "", nil, false
	}

	name, opts, _ := strings.Cut(tag, ",")
	if name == "" {
		if fmtType == formatYAML {
			name = strings.ToLower(field.Name)
		} else {
			name = field.Name
		}
	}
	if opts == "" {
		return name, nil, true
	}
	return name, strings.Split(opts, ","), true
}

func isSecretField(field reflect.StructField) bool {
	for _, tag := range strings.Split(field.Tag.Get(RedactTagName), ",") {
		if strings.TrimSpace(tag) == RedactTagSecret {
			return true
		}
	}
	return false
}

func toPlain(fmtType format, v any) (any, error) {
	if v == nil {
		return nil, nil
	}

	var data []byte
	var err error
	if fmtType == formatYAML {
		data, err = yaml.Marshal(v)
	} else {
		data, err = json.Marshal(v)
	}
	if err != nil {
		return nil, err
	}

	var plain any
	if fmtType == formatYAML {
		if err := yaml.Unmarshal(data, &plain); err != nil {
			return nil, err
		}
		return normalizeYAML(plain), nil
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&plain); err != nil {
		return nil, err
	}
	return plain, nil
}

func normalizeYAML(v any) any {
	switch typed := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, value := range typed {
			out[key] = normalizeYAML(value)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(typed))
		for key, value := range typed {
			out[fmt.Sprint(key)] = normalizeYAML(value)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, value := range typed {
			out[i] = normalizeYAML(value)
		}
		return out
	default:
		return typed
	}
}

type visit struct {
	typ reflect.Type
	ptr uintptr
}

func valueHasRedactionTag(v reflect.Value, seen map[visit]bool) bool {
	if !v.IsValid() {
		return false
	}

	v = unwrapInterface(v)
	if !v.IsValid() || isNil(v) {
		return false
	}

	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return false
		}
		id := visit{typ: v.Type(), ptr: v.Pointer()}
		if seen[id] {
			return false
		}
		seen[id] = true
		v = unwrapInterface(v.Elem())
	}

	if inner, ok := innerValue(v); ok {
		return valueHasRedactionTag(inner, seen)
	}

	switch v.Kind() {
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if field.PkgPath != "" && !field.Anonymous {
				continue
			}
			if isSecretField(field) {
				return true
			}
			if valueHasRedactionTag(v.Field(i), seen) {
				return true
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if valueHasRedactionTag(v.Index(i), seen) {
				return true
			}
		}
	case reflect.Map:
		iter := v.MapRange()
		for iter.Next() {
			if valueHasRedactionTag(iter.Value(), seen) {
				return true
			}
		}
	}

	return false
}

func innerValue(v reflect.Value) (reflect.Value, bool) {
	if v.Kind() != reflect.Struct {
		return reflect.Value{}, false
	}
	field := v.FieldByName("InnerVal")
	if !field.IsValid() || !canBeNil(field) || field.IsNil() {
		return reflect.Value{}, false
	}
	return unwrapInterface(field), true
}

func unwrapInterface(v reflect.Value) reflect.Value {
	for v.IsValid() && v.Kind() == reflect.Interface {
		if v.IsNil() {
			return reflect.Value{}
		}
		v = v.Elem()
	}
	return v
}

func isNil(v reflect.Value) bool {
	if !canBeNil(v) {
		return false
	}
	return v.IsNil()
}

func canBeNil(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return true
	default:
		return false
	}
}

func mapKeyToString(v reflect.Value) (string, error) {
	if v.Kind() == reflect.String {
		return v.String(), nil
	}
	if v.CanInterface() {
		if tm, ok := v.Interface().(encoding.TextMarshaler); ok {
			text, err := tm.MarshalText()
			return string(text), err
		}
	}
	return fmt.Sprint(v.Interface()), nil
}

func isEmptyValue(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Pointer:
		return v.IsNil()
	}
	return false
}
