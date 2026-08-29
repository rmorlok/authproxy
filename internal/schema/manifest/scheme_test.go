package manifest

import (
	"errors"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
)

type widget struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      meta.ObjectMeta `json:"metadata" yaml:"metadata"`
	Spec          widgetSpec      `json:"spec" yaml:"spec"`
}

type widgetSpec struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
}

var widgetGVK = GVK{APIVersion: meta.APIVersionV1Alpha1, Kind: "Widget"}

func newWidgetScheme(t *testing.T) *Scheme {
	t.Helper()
	scheme := NewScheme()
	require.NoError(t, RegisterType[widget](scheme, widgetGVK))
	return scheme
}

func TestDecodeJSONStrictDispatch(t *testing.T) {
	scheme := newWidgetScheme(t)
	decoded, err := scheme.DecodeJSON([]byte(`{
      "apiVersion":"authproxy.net/v1alpha1",
      "kind":"Widget",
      "metadata":{"name":"example"},
      "spec":{"enabled":true}
    }`))
	require.NoError(t, err)
	resource, ok := decoded.(*widget)
	require.True(t, ok)
	require.Equal(t, meta.Kind("Widget"), resource.Kind)
	require.Equal(t, common.ResourceName("example"), resource.Metadata.Name)
	require.True(t, resource.Spec.Enabled)

	_, err = scheme.DecodeJSON([]byte(`{
      "apiVersion":"authproxy.net/v1alpha1",
      "kind":"Widget",
      "metadata":{"name":"example"},
      "spec":{"enabled":true,"unknown":1}
    }`))
	require.ErrorContains(t, err, "unknown field")

	_, err = scheme.DecodeJSON([]byte(`{"apiVersion":"authproxy.net/v1alpha1","kind":"Widget","metadata":{},"spec":{}} {}`))
	require.ErrorContains(t, err, "unexpected additional JSON value")
}

func TestUnknownAndMismatchedGVKsAreFieldAddressable(t *testing.T) {
	scheme := newWidgetScheme(t)
	tests := []struct {
		name string
		data string
		path string
	}{
		{"missing version", `{"kind":"Widget"}`, "$.apiVersion"},
		{"missing kind", `{"apiVersion":"authproxy.net/v1alpha1"}`, "$.kind"},
		{"unknown version", `{"apiVersion":"authproxy.net/v1alpha2","kind":"Widget"}`, "$.apiVersion"},
		{"unknown kind", `{"apiVersion":"authproxy.net/v1alpha1","kind":"Gadget"}`, "$.kind"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := scheme.DecodeJSON([]byte(test.data))
			require.Error(t, err)
			var fieldError *common.ValidationError
			require.True(t, errors.As(err, &fieldError), "expected field-addressable error, got %T: %v", err, err)
			require.Equal(t, test.path, fieldError.Path)
		})
	}

	_, err := scheme.DecodeYAML([]byte("apiVersion: authproxy.net/v1alpha1\nkind: Gadget\n"))
	require.Error(t, err)
	var fieldError *common.ValidationError
	require.True(t, errors.As(err, &fieldError))
	require.Equal(t, "$.kind", fieldError.Path)
}

func TestDecodeYAMLAndMultiDocumentStream(t *testing.T) {
	scheme := newWidgetScheme(t)
	stream := []byte(`
apiVersion: authproxy.net/v1alpha1
kind: Widget
metadata:
  name: first
spec:
  enabled: true
---
---
apiVersion: authproxy.net/v1alpha1
kind: Widget
metadata:
  name: second
spec:
  enabled: false
`)
	resources, err := scheme.DecodeYAMLDocuments(stream)
	require.NoError(t, err)
	require.Len(t, resources, 2)
	require.Equal(t, common.ResourceName("first"), resources[0].(*widget).Metadata.Name)
	require.Equal(t, common.ResourceName("second"), resources[1].(*widget).Metadata.Name)

	_, err = scheme.DecodeYAML(stream)
	require.ErrorContains(t, err, "expected one document, got 2")

	decoded, err := scheme.DecodeYAML([]byte(`
apiVersion: authproxy.net/v1alpha1
kind: Widget
metadata:
  name: one
spec:
  enabled: true
`))
	require.NoError(t, err)
	require.Equal(t, common.ResourceName("one"), decoded.(*widget).Metadata.Name)

	_, err = scheme.DecodeYAML([]byte(`
apiVersion: authproxy.net/v1alpha1
kind: Widget
metadata: {}
spec:
  enabled: true
  unknown: value
`))
	require.ErrorContains(t, err, "field unknown not found")
}

func TestSchemeRegistration(t *testing.T) {
	scheme := newWidgetScheme(t)
	require.Equal(t, []GVK{widgetGVK}, scheme.RegisteredGVKs())
	require.ErrorContains(t, RegisterType[widget](scheme, widgetGVK), "already registered")
	require.ErrorContains(t, scheme.Register(GVK{APIVersion: meta.APIVersionV1Alpha1, Kind: "Value"}, func() any { return widget{} }), "must return a pointer")
	require.ErrorContains(t, scheme.Register(GVK{APIVersion: meta.APIVersionV1Alpha1, Kind: "NilValue"}, func() any { return (*widget)(nil) }), "returned a nil")
	require.ErrorContains(t, scheme.Register(GVK{APIVersion: "not-a-version", Kind: "Value"}, func() any { return &widget{} }), "apiVersion")
}
