package util

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestYamlBytesToJSON(t *testing.T) {
	t.Parallel()
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

func TestYamlDocumentEmpty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		node  *yaml.Node
		empty bool
	}{
		{
			name:  "nil node",
			empty: true,
		},
		{
			name:  "document without content",
			node:  &yaml.Node{Kind: yaml.DocumentNode},
			empty: true,
		},
		{
			name: "implicit null scalar",
			node: &yaml.Node{
				Kind: yaml.DocumentNode,
				Content: []*yaml.Node{
					{Kind: yaml.ScalarNode, Tag: "!!null", Value: " \n"},
				},
			},
			empty: true,
		},
		{
			name: "explicit null scalar",
			node: &yaml.Node{
				Kind: yaml.DocumentNode,
				Content: []*yaml.Node{
					{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"},
				},
			},
			empty: false,
		},
		{
			name: "empty string scalar",
			node: &yaml.Node{
				Kind: yaml.DocumentNode,
				Content: []*yaml.Node{
					{Kind: yaml.ScalarNode, Tag: "!!str", Value: ""},
				},
			},
			empty: false,
		},
		{
			name: "mapping node",
			node: &yaml.Node{
				Kind: yaml.DocumentNode,
				Content: []*yaml.Node{
					{Kind: yaml.MappingNode, Tag: "!!map"},
				},
			},
			empty: false,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, test.empty, YamlDocumentEmpty(test.node))
		})
	}
}

func TestYamlDocumentEmptyWithDecodedYAML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		data  string
		empty bool
	}{
		{name: "empty input", data: "", empty: true},
		{name: "whitespace", data: "  \n", empty: true},
		{name: "comment only", data: "# comment\n", empty: true},
		{name: "empty document marker", data: "---\n", empty: true},
		{name: "explicit null", data: "null\n", empty: false},
		{name: "null shorthand", data: "~\n", empty: false},
		{name: "quoted empty string", data: `""`, empty: false},
		{name: "empty mapping", data: "{}\n", empty: false},
		{name: "empty sequence", data: "[]\n", empty: false},
		{name: "scalar", data: "value\n", empty: false},
		{name: "mapping", data: "key: value\n", empty: false},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var node yaml.Node
			require.NoError(t, yaml.Unmarshal([]byte(test.data), &node))
			require.Equal(t, test.empty, YamlDocumentEmpty(&node))
		})
	}
}
