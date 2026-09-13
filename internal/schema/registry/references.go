package registry

import (
	"reflect"
	"sort"

	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
)

// References discovers canonical ObjectReference fields in a resource. It
// follows typed containers and polymorphic wrappers, rather than guessing from
// JSON key names or maintaining a resource-specific catalogue of reference paths.
// Returned references are values detached from the resource.
func References(resource any) ([]meta.ObjectReference, error) {
	if _, err := TypeOf(resource); err != nil {
		return nil, err
	}
	var result []meta.ObjectReference
	referenceType := reflect.TypeOf(meta.ObjectReference{})
	type visit struct {
		typ reflect.Type
		ptr uintptr
	}
	seen := map[visit]bool{}
	var walk func(reflect.Value)
	walk = func(v reflect.Value) {
		if !v.IsValid() {
			return
		}
		if v.Kind() == reflect.Interface {
			if !v.IsNil() {
				walk(v.Elem())
			}
			return
		}
		if v.Kind() == reflect.Pointer || v.Kind() == reflect.Map || v.Kind() == reflect.Slice {
			if v.IsNil() {
				return
			}
			key := visit{v.Type(), v.Pointer()}
			if seen[key] {
				return
			}
			seen[key] = true
			defer delete(seen, key)
		}
		if v.Kind() == reflect.Pointer {
			walk(v.Elem())
			return
		}
		if v.Type() == referenceType {
			if v.IsZero() {
				return
			} // Omitted value fields, such as Connection.spec.connectorRef.
			result = append(result, v.Interface().(meta.ObjectReference))
			return
		}
		switch v.Kind() {
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				f := v.Type().Field(i)
				if f.PkgPath != "" {
					continue
				}
				if f.Tag.Get("json") == "-" && f.Name != "InnerVal" {
					continue
				}
				walk(v.Field(i))
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < v.Len(); i++ {
				walk(v.Index(i))
			}
		case reflect.Map:
			// Resource maps use string keys; sorting makes discovery deterministic.
			keys := v.MapKeys()
			sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
			for _, k := range keys {
				walk(v.MapIndex(k))
			}
		}
	}
	walk(reflect.ValueOf(resource))
	return result, nil
}
