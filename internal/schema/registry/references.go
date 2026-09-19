package registry

import (
	"reflect"
	"sort"

	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
)

// referencesResourceTypeValidator is the validator used by the References
// function to establish that the resource is a valid type. This is to gate
// the function to only be able to use authproxy resource types, but allow
// that gate to be bypassed for testing for scenarios that don't currently
// exist in the current resource set.
var referencesResourceTypeValidator = func(resource any) error {
	// Make sure the object is of registered resource type
	_, err := TypeOf(resource)
	return err
}

// References identifies the other resources referenced from a given resource
// in the form of ObjectReferences. It does this via reflection from the typed
// structs to avoid a hard-coded list per-resoruce type. It follows typed
// containers and polymorphic wrappers. Returned references are values detached
// from the resource.
func References(resource any) ([]meta.ObjectReference, error) {
	if err := referencesResourceTypeValidator(resource); err != nil {
		return nil, err
	}

	// The set of references we found.
	var result []meta.ObjectReference

	// The type we are looking for.
	referenceType := reflect.TypeOf(meta.ObjectReference{})

	type visit struct {
		typ reflect.Type
		ptr uintptr
	}

	// Track visits to pointer related types to avoid walking in cycles.
	seen := map[visit]bool{}

	var walk func(reflect.Value)
	walk = func(v reflect.Value) {
		if !v.IsValid() {
			return
		}

		// Step into interface values.
		if v.Kind() == reflect.Interface {
			if !v.IsNil() {
				walk(v.Elem())
			}
			return
		}

		if v.Kind() == reflect.Pointer ||
			v.Kind() == reflect.Map ||
			v.Kind() == reflect.Slice {
			if v.IsNil() {
				return
			}

			key := visit{v.Type(), v.Pointer()}

			// Make sure we aren't walking in a cycle.
			if seen[key] {
				return
			}

			seen[key] = true
			defer delete(seen, key)
		}

		// Step into pointer values.
		if v.Kind() == reflect.Pointer {
			walk(v.Elem())
			return
		}

		if v.Type() == referenceType {
			if v.IsZero() {
				// Omitted value fields, such as Connection.spec.connectorRef.
				return
			}

			// Found a reference. Add it to our tracking list.
			result = append(result, v.Interface().(meta.ObjectReference))

			return
		}

		switch v.Kind() {
		// Walk over all fields of the struct
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				f := v.Type().Field(i)
				if f.PkgPath != "" {
					continue
				}

				// Skip fields that don't render to json or have an InnerVal,
				// which our approach to implementing polymorphic types.
				if f.Tag.Get("json") == "-" && f.Name != "InnerVal" {
					continue
				}

				walk(v.Field(i))
			}

		// Walk over all elements of the slice or array
		case reflect.Slice, reflect.Array:
			for i := 0; i < v.Len(); i++ {
				walk(v.Index(i))
			}

		// Walk over all elements of a map in a deterministic order
		case reflect.Map:
			// Resource maps use string keys; sorting makes discovery deterministic.
			keys := v.MapKeys()
			sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
			for _, k := range keys {
				walk(v.MapIndex(k))
			}
		}
	}

	// Start teh walk at the root resource
	walk(reflect.ValueOf(resource))


	return result, nil
}
