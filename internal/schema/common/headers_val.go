package common

import (
	"encoding/json"
	"fmt"
)

// HeadersVal is a proxy request header value as it appears in JSON.
// It accepts either a single string or an array of strings so callers can
// represent repeated HTTP field values without losing order.
type HeadersVal struct {
	values []string
	array  bool
}

func NewHeadersVal(value string) HeadersVal {
	return HeadersVal{values: []string{value}}
}

func NewHeadersValSlice(values []string) HeadersVal {
	return HeadersVal{values: append([]string(nil), values...), array: true}
}

func HeadersValMapFromStrings(headers map[string]string) map[string]HeadersVal {
	if headers == nil {
		return nil
	}
	out := make(map[string]HeadersVal, len(headers))
	for k, v := range headers {
		out[k] = NewHeadersVal(v)
	}
	return out
}

func (v HeadersVal) Values() []string {
	return append([]string(nil), v.values...)
}

func (v HeadersVal) MarshalJSON() ([]byte, error) {
	if v.array {
		return json.Marshal(v.values)
	}
	if len(v.values) == 0 {
		return json.Marshal("")
	}
	return json.Marshal(v.values[0])
}

func (v *HeadersVal) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		v.values = []string{single}
		v.array = false
		return nil
	}

	var multiple []string
	if err := json.Unmarshal(data, &multiple); err == nil {
		v.values = append([]string(nil), multiple...)
		v.array = true
		return nil
	}

	return fmt.Errorf("header value must be a string or array of strings")
}
