package apserde

import (
	"reflect"
	"strconv"
)

// SecretPaths returns wire-format JSON path segments for schema-tagged secret
// fields, including null and empty fields. Values are never included. Paths
// through arrays use decimal indexes. Callers must also exclude write-only
// contracts whose entire configuration must not be persisted.
func SecretPaths(value any) [][]string {
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
				if isSecretField(f) {
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
