package toolsets

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util"
)

// decodeStrictObject checks owned canonical keys and explicit nulls before
// pointer decoding could erase their presence. label names the object in
// errors; required fields are checked by Validate after decoding a complete value.
func decodeStrictObject(data []byte, label string, allowed ...string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := util.DecodeJSONStrict(data, &fields); err != nil {
		return nil, err
	}

	if fields == nil {
		return nil, fmt.Errorf("%s must be an object", label)
	}

	for field, raw := range fields {
		if !slices.Contains(allowed, field) {
			return nil, fmt.Errorf("unknown %s field %q", label, field)
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil, fmt.Errorf("%s must not be null", field)
		}
	}

	return fields, nil
}

// jsonFieldNames returns the canonical JSON names of a struct type's exported
// fields, so strict decoders stay in sync with the type they guard.
func jsonFieldNames(t reflect.Type) []string {
	names := make([]string, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		if name == "" {
			name = field.Name
		}
		names = append(names, name)
	}
	return names
}

// exactNameLists is an include/exclude pair of exact upstream identifiers, such
// as MCP tool names or OpenAPI operation IDs. Omitted inclusion is unrestricted;
// a supplied inclusion must be nonempty. Exclusions win overlap, and names are
// never trimmed or normalized. Nil pointers are omitted, while supplied lists
// must point to non-nil slices.
type exactNameLists struct {
	includeField string
	include      *[]string
	excludeField string
	exclude      *[]string
}

// validate rejects supplied nil slices and checks nonblank, per-list unique
// names and a nonempty inclusion.
func (l exactNameLists) validate(vc *common.ValidationContext) error {
	vc = validationContext(vc)

	var result *multierror.Error

	for _, list := range []struct {
		field string
		names *[]string
	}{
		{l.includeField, l.include},
		{l.excludeField, l.exclude},
	} {
		if list.names == nil {
			continue
		}
		if *list.names == nil {
			result = multierror.Append(result, vc.NewErrorForField(list.field, "must not be null"))
			continue
		}
		if list.field == l.includeField && len(*list.names) == 0 {
			result = multierror.Append(result, vc.NewErrorForField(list.field, "must not be empty when supplied"))
		}
		seen := make(map[string]bool, len(*list.names))

		for i, name := range *list.names {
			path := vc.PushField(list.field).PushIndex(i)

			if strings.TrimSpace(name) == "" {
				result = multierror.Append(result, path.NewError("must not be blank"))
			}

			if seen[name] {
				result = multierror.Append(result, path.NewError("must be unique within the list"))
			}

			seen[name] = true
		}
	}

	return result.ErrorOrNil()
}

// decode fills both lists from a strict JSON object, retaining list presence
// and rejecting null elements instead of turning them into empty names. The
// receiver's field names select the accepted keys; label names it in errors.
func (l *exactNameLists) decode(data []byte, label string) error {
	fields, err := decodeStrictObject(data, label, l.includeField, l.excludeField)
	if err != nil {
		return err
	}

	l.include, l.exclude = nil, nil

	for field, raw := range fields {
		var names []*string
		if err := util.DecodeJSONStrict(raw, &names); err != nil {
			return fmt.Errorf("decode %s: %w", field, err)
		}

		values := make([]string, len(names))
		for i, name := range names {
			if name == nil {
				return fmt.Errorf("%s[%d] must be a string, not null", field, i)
			}
			values[i] = *name
		}

		if field == l.includeField {
			l.include = &values
		} else {
			l.exclude = &values
		}
	}

	return nil
}
