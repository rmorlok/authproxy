package apserde

import (
	"reflect"
	"strconv"
)

// WriteOnlyTagName marks canonical fields whose full configuration is not
// returned by ordinary resource reads. Keep it aligned with JSON Schema's
// writeOnly annotation. This metadata does not change API masking or replay.
const WriteOnlyTagName = "apiwriteonly"

// SecretPaths returns wire-format JSON path segments for schema-tagged secret
// fields, including null and empty fields. Values are never included. Paths
// through arrays use decimal indexes.
func SecretPaths(value any) [][]string {
	return fieldPaths(value, isSecretField)
}

// WriteOnlyPaths discovers entire write-only fields, including null fields.
// Unlike secret masking, this also covers non-secret provider configuration
// that cannot be compared with an ordinary resource read.
func WriteOnlyPaths(value any) [][]string {
	return fieldPaths(value, func(field reflect.StructField) bool {
		return field.Tag.Get(WriteOnlyTagName) == "true"
	})
}

// SensitivePaths includes both masked secrets and entire write-only contracts.
// Consumers such as apply history must exclude these values, not hash or mask
// them. Paths are discovered from the canonical fields without resource switches.
func SensitivePaths(value any) [][]string {
	return append(SecretPaths(value), WriteOnlyPaths(value)...)
}

func fieldPaths(value any, matches func(reflect.StructField) bool) [][]string {
	var paths [][]string
	seen := map[visit]bool{}

	var walk func(reflect.Value, []string)
	walk = func(v reflect.Value, path []string) {
		v = unwrapInterface(v)

		if !v.IsValid() || isNil(v) {
			return
		}

		if v.Kind() == reflect.Pointer ||
			v.Kind() == reflect.Map ||
			v.Kind() == reflect.Slice {

			key := visit{typ: v.Type(), ptr: v.Pointer()}

			if seen[key] {
				return
			}
			seen[key] = true
			defer delete(seen, key)
		}

		for v.Kind() == reflect.Pointer {
			v = unwrapInterface(v.Elem())
			if !v.IsValid() {
				return
			}
		}

		if inner, ok := innerValue(v); ok {
			walk(inner, path)
			return
		}

		switch v.Kind() {
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				f := v.Type().Field(i)
				if f.PkgPath != "" && !f.Anonymous {
					continue
				}
				name, _, ok := fieldName(formatJSON, f)
				if !ok {
					continue
				}
				next := append(append([]string(nil), path...), name)
				if fieldIsInline(formatJSON, f) {
					next = path
				}
				if matches(f) {
					paths = append(paths, next)
				} else {
					walk(v.Field(i), next)
				}
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < v.Len(); i++ {
				walk(v.Index(i), append(append([]string(nil), path...), strconv.Itoa(i)))
			}
		case reflect.Map:
			iter := v.MapRange()
			for iter.Next() {
				key, err := mapKeyToString(iter.Key())
				if err == nil {
					walk(iter.Value(), append(append([]string(nil), path...), key))
				}
			}
		}
	}

	walk(reflect.ValueOf(value), nil)
	return paths
}
