package util

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestYamlBytesToJSON(t *testing.T) {
	type TestData struct {
		Foo string `json:"foo" yaml:"foo"`
		Bar struct {
			Baz int64 `json:"baz" yaml:"baz"`
		} `json:"bar" yaml:"bar"`
	}

	testData := &TestData{
		Foo: "bob dole",
		Bar: struct {
			Baz int64 `json:"baz" yaml:"baz"`
		}{
			Baz: 1234567890,
		},
	}

	yamlBytes, err := yaml.Marshal(testData)
	require.NoError(t, err)

	jsonBytes, err := YamlBytesToJSON(yamlBytes)
	require.NoError(t, err)

	var resultData TestData
	err = json.Unmarshal(jsonBytes, &resultData)
	require.NoError(t, err)

	require.Equal(t, *testData, resultData)
}
