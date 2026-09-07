package apserde

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

var errTestMarshal = errors.New("test marshal failure")

type apiFailingJSONMarshaler struct{}

func (apiFailingJSONMarshaler) MarshalJSON() ([]byte, error) {
	return nil, errTestMarshal
}

type apiFailingYAMLMarshaler struct{}

func (apiFailingYAMLMarshaler) MarshalYAML() (any, error) {
	return nil, errTestMarshal
}

type apiFailingTextKey int

func (apiFailingTextKey) MarshalText() ([]byte, error) {
	return nil, errTestMarshal
}

type apiEmptyStringer struct{}

func (apiEmptyStringer) String() string {
	return ""
}

func TestSecretReplayContext(t *testing.T) {
	require.False(t, SecretReplayAllowed(nil))
	require.False(t, SecretReplayAllowed(context.Background()))
	require.False(t, SecretReplayAllowed(WithSecretReplay(context.Background(), false)))
	require.True(t, SecretReplayAllowed(WithSecretReplay(context.Background(), true)))
	require.False(t, SecretReplayAllowed(context.WithValue(context.Background(), secretReplayKey, "true")))
}

func TestSanitizeForAPIFastPathAndErrors(t *testing.T) {
	plain, report, err := SanitizeJSONForAPI(nil, nil)
	require.NoError(t, err)
	require.Nil(t, plain)
	require.False(t, report.Redacted)

	plain, report, err = SanitizeYAMLForAPI(context.Background(), struct {
		Value string `yaml:"value"`
	}{Value: "plain"})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"value": "plain"}, plain)
	require.False(t, report.Redacted)

	_, _, err = SanitizeJSONForAPI(context.Background(), apiFailingJSONMarshaler{})
	require.ErrorIs(t, err, errTestMarshal)
	_, _, err = MarshalJSONForAPI(context.Background(), apiFailingJSONMarshaler{})
	require.ErrorIs(t, err, errTestMarshal)

	_, _, err = SanitizeYAMLForAPI(context.Background(), apiFailingYAMLMarshaler{})
	require.ErrorIs(t, err, errTestMarshal)
	_, _, err = MarshalYAMLForAPI(context.Background(), apiFailingYAMLMarshaler{})
	require.ErrorIs(t, err, errTestMarshal)
}

func TestMarshalJSONForAPIDisablesHTMLEscaping(t *testing.T) {
	data, report, err := MarshalJSONForAPI(context.Background(), apiSecretPayload{Public: "<visible>"})
	require.NoError(t, err)
	require.False(t, report.Redacted)
	require.Contains(t, string(data), `"public":"<visible>"`)
}

func TestMaskPlain(t *testing.T) {
	input := map[string]any{
		"text":   "sëcret",
		"nested": []any{12, "", nil},
	}

	masked, didMask, err := maskPlain(input)
	require.NoError(t, err)
	require.True(t, didMask)
	require.Equal(t, map[string]any{
		"text":   "******",
		"nested": []any{"**", "", nil},
	}, masked)
	require.Equal(t, map[string]any{
		"text":   "sëcret",
		"nested": []any{12, "", nil},
	}, input, "masking must not mutate the source value")

	masked, didMask, err = maskPlain(apiEmptyStringer{})
	require.NoError(t, err)
	require.False(t, didMask)
	require.Equal(t, apiEmptyStringer{}, masked)
}

func TestRedactValueHandlesNilAndMarshalErrors(t *testing.T) {
	masked, didMask, err := redactValue(formatJSON, reflect.Value{})
	require.NoError(t, err)
	require.Nil(t, masked)
	require.False(t, didMask)

	var nilValue *string
	masked, didMask, err = redactValue(formatJSON, reflect.ValueOf(nilValue))
	require.NoError(t, err)
	require.Nil(t, masked)
	require.False(t, didMask)

	_, _, err = redactValue(formatJSON, reflect.ValueOf(apiFailingJSONMarshaler{}))
	require.ErrorIs(t, err, errTestMarshal)
}

func TestSanitizeValueEdgeCases(t *testing.T) {
	report := Report{}
	plain, err := sanitizeValue(formatJSON, reflect.Value{}, &report)
	require.NoError(t, err)
	require.Nil(t, plain)

	var nilValue *apiSecretPayload
	plain, err = sanitizeValue(formatJSON, reflect.ValueOf(nilValue), &report)
	require.NoError(t, err)
	require.Nil(t, plain)

	_, err = sanitizeValue(formatJSON, reflect.ValueOf(map[apiFailingTextKey]apiSecretPayload{
		1: {Secret: "secret"},
	}), &report)
	require.ErrorIs(t, err, errTestMarshal)

	type invalidInlinePayload struct {
		Inline string `json:",inline"`
		Secret string `json:"secret" apiredact:"secret"`
	}
	_, err = sanitizeValue(formatJSON, reflect.ValueOf(invalidInlinePayload{
		Inline: "not-an-object",
		Secret: "secret",
	}), &report)
	require.EqualError(t, err, "inline field Inline must serialize to an object")
}

func TestValidateNoRedactedPlaceholdersTraversesContainers(t *testing.T) {
	type recursivePayload struct {
		APIEmbeddedMeta `json:",inline"`
		Secret          string                       `json:"secret" apiredact:"secret"`
		Next            *recursivePayload            `json:"next,omitempty"`
		Items           []apiSecretPayload           `json:"items"`
		Lookup          map[string]*apiSecretPayload `json:"lookup"`
		Wrapped         apiWrapper                   `json:"wrapped"`
		Array           [1]apiSecretPayload          `json:"array"`
		Ignored         apiSecretPayload             `json:"-"`
	}

	shared := &apiSecretPayload{Secret: "***"}
	payload := &recursivePayload{
		APIEmbeddedMeta: APIEmbeddedMeta{Kind: "Example"},
		Secret:          "real-secret",
		Items:           []apiSecretPayload{{Secret: "***"}},
		Lookup:          map[string]*apiSecretPayload{"one": shared, "alias": shared},
		Wrapped:         apiWrapper{InnerVal: apiInnerSecret{Secret: "***"}},
		Array:           [1]apiSecretPayload{{Secret: "***"}},
		Ignored:         apiSecretPayload{Secret: "***"},
	}
	payload.Next = payload

	err := ValidateNoRedactedPlaceholders(payload)
	require.Error(t, err)
	require.Contains(t, err.Error(), "$.items[0].secret")
	require.Contains(t, err.Error(), `$.lookup["one"].secret`)
	require.Contains(t, err.Error(), `$.lookup["alias"].secret`)
	require.Contains(t, err.Error(), "$.wrapped.client_secret")
	require.Contains(t, err.Error(), "$.array[0].secret")
	require.NotContains(t, err.Error(), "ignored")
}

func TestValidateNoRedactedPlaceholdersHandlesPlainAndErrorValues(t *testing.T) {
	require.NoError(t, ValidateNoRedactedPlaceholders(nil))
	require.NoError(t, ValidateNoRedactedPlaceholders("***"))

	type failingSecret struct {
		Secret apiFailingJSONMarshaler `json:"secret" apiredact:"secret"`
	}
	require.ErrorIs(t, ValidateNoRedactedPlaceholders(failingSecret{}), errTestMarshal)

	type failingMap struct {
		Values map[apiFailingTextKey]apiSecretPayload `json:"values"`
	}
	require.ErrorIs(t, ValidateNoRedactedPlaceholders(failingMap{
		Values: map[apiFailingTextKey]apiSecretPayload{1: {Secret: "***"}},
	}), errTestMarshal)
}

func TestToPlainAndNormalizeYAML(t *testing.T) {
	plain, err := toPlain(formatJSON, struct {
		Number int `json:"number"`
	}{Number: 42})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"number": json.Number("42")}, plain)

	plain, err = toPlain(formatYAML, map[any]any{
		1: []any{map[any]any{"nested": 2}},
	})
	require.NoError(t, err)
	require.Equal(t, map[string]any{
		"1": []any{map[string]any{"nested": 2}},
	}, plain)

	require.Nil(t, normalizeYAML(nil))
	require.Equal(t, "scalar", normalizeYAML("scalar"))

	_, err = toPlain(formatJSON, apiFailingJSONMarshaler{})
	require.ErrorIs(t, err, errTestMarshal)
	_, err = toPlain(formatYAML, apiFailingYAMLMarshaler{})
	require.ErrorIs(t, err, errTestMarshal)
}

func TestValueHasRedactionTag(t *testing.T) {
	type cycle struct {
		Next *cycle
	}
	type privateSecret struct {
		secret string `apiredact:"secret"`
	}

	require.False(t, valueHasRedactionTag(reflect.Value{}, map[visit]bool{}))
	var nilPayload *apiSecretPayload
	require.False(t, valueHasRedactionTag(reflect.ValueOf(nilPayload), map[visit]bool{}))
	require.True(t, valueHasRedactionTag(reflect.ValueOf(apiSecretPayload{}), map[visit]bool{}))
	require.True(t, valueHasRedactionTag(reflect.ValueOf([]apiSecretPayload{{}}), map[visit]bool{}))
	require.True(t, valueHasRedactionTag(reflect.ValueOf([1]apiSecretPayload{{}}), map[visit]bool{}))
	require.True(t, valueHasRedactionTag(reflect.ValueOf(map[string]apiSecretPayload{"one": {}}), map[visit]bool{}))
	require.True(t, valueHasRedactionTag(reflect.ValueOf(apiWrapper{InnerVal: apiInnerSecret{}}), map[visit]bool{}))
	require.False(t, valueHasRedactionTag(reflect.ValueOf(privateSecret{}), map[visit]bool{}))

	cyclic := &cycle{}
	cyclic.Next = cyclic
	require.False(t, valueHasRedactionTag(reflect.ValueOf(cyclic), map[visit]bool{}))
}
