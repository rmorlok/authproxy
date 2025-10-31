package common

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/rmorlok/authproxy/util"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestStringValue(t *testing.T) {
	t.Run("round trip starting from objects", func(t *testing.T) {
		tests := []struct {
			name string
			Val  StringValueType
		}{
			{
				name: "inline direct value",
				Val: &StringValueDirect{
					Value:          "https://example.com/some.png",
					IsDirectString: true,
				},
			},
			{
				name: "inline direct",
				Val: &StringValueDirect{
					Value:          "https://example.com/some.png",
					IsDirectString: false,
				},
			},
			{
				name: "base64",
				Val: &StringValueBase64{
					Base64: "ywAAAAAAQABAAACAUwAOw==",
				},
			},
			{
				name: "env var",
				Val: &StringValueEnvVar{
					EnvVar: "SOME_ENV_VAR",
				},
			},
			{
				name: "env var - default",
				Val: &StringValueEnvVar{
					EnvVar:  "SOME_ENV_VAR",
					Default: util.ToPtr("some default"),
				},
			},
			{
				name: "env var base64",
				Val: &StringValueEnvVarBase64{
					EnvVar: "SOME_ENV_VAR",
				},
			},
			{
				name: "env var base64 - default",
				Val: &StringValueEnvVarBase64{
					EnvVar:  "SOME_ENV_VAR",
					Default: util.ToPtr("ywAAAAAAQABAAACAUwAOw=="),
				},
			},
			{
				name: "file",
				Val: &StringValueFile{
					Path: "/some/file",
				},
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				t.Run("yaml", func(t *testing.T) {
					wrapper := &StringValue{test.Val}
					data, err := yaml.Marshal(wrapper)
					require.NoError(t, err)

					var output StringValue
					err = yaml.Unmarshal(data, &output)
					require.NoError(t, err)
					require.Equal(t, test.Val, output.Inner())
				})
				t.Run("json", func(t *testing.T) {
					wrapper := &StringValue{test.Val}
					data, err := json.Marshal(wrapper)
					require.NoError(t, err)

					var output StringValue
					err = json.Unmarshal(data, &output)
					require.NoError(t, err)
					require.Equal(t, test.Val, output.Inner())
				})
			})
		}
	})

	t.Run("yaml", func(t *testing.T) {
		t.Run("roundtrip", func(t *testing.T) {
			tests := []struct {
				name     string
				data     string
				expected StringValueType
			}{
				{
					name: "inline direct value",
					expected: &StringValueDirect{
						Value:          "some value",
						IsDirectString: true,
					},
					data: `some value`,
				},
				{
					name: "direct value",
					expected: &StringValueDirect{
						Value:          "some value",
						IsDirectString: false,
					},
					data: `
value: some value
`,
				},
				{
					name: "base64",
					expected: &StringValueBase64{
						Base64: "ywAAAAAAQABAAACAUwAOw==",
					},
					data: `
base64: ywAAAAAAQABAAACAUwAOw==
`,
				},
			}

			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					var val StringValue
					err := yaml.Unmarshal([]byte(test.data), &val)
					require.NoError(t, err)
					require.Equal(t, test.expected, val.Inner())
					require.Equal(t, strings.TrimSpace(test.data), strings.TrimSpace(MustMarshalToYamlString(val.Inner())))
				})
			}
		})
		t.Run("parse", func(t *testing.T) {
			t.Run("inline direct value", func(t *testing.T) {
				data := `some value
`
				var val StringValue
				err := yaml.Unmarshal([]byte(data), &val)
				require.NoError(t, err)
				require.Equal(t, &StringValueDirect{
					Value:          "some value",
					IsDirectString: true,
				}, val.Inner())
			})
			t.Run("direct value", func(t *testing.T) {
				data := `
value: some value
`
				var val StringValue
				err := yaml.Unmarshal([]byte(data), &val)
				require.NoError(t, err)
				require.Equal(t, &StringValueDirect{
					Value:          "some value",
					IsDirectString: false,
				}, val.Inner())
			})
			t.Run("base64", func(t *testing.T) {
				data := `
base64: iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/wcAAwAB/1J8qkwAAAAASUVORK5CYII=`
				var val StringValue
				err := yaml.Unmarshal([]byte(data), &val)
				require.NoError(t, err)
				require.Equal(t, &StringValueBase64{
					Base64: "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/wcAAwAB/1J8qkwAAAAASUVORK5CYII=",
				}, val.Inner())
			})
		})

		t.Run("yaml gen", func(t *testing.T) {
			t.Run("inline value direct", func(t *testing.T) {
				data := &StringValueDirect{
					Value:          "https://example.com/some.png",
					IsDirectString: true,
				}
				require.Equal(t, "https://example.com/some.png\n", MustMarshalToYamlString(data))
			})
			t.Run("value direct", func(t *testing.T) {
				data := &StringValueDirect{
					Value: "https://example.com/some.png",
				}
				require.Equal(t, "value: https://example.com/some.png\n", MustMarshalToYamlString(data))
			})
			t.Run("base64", func(t *testing.T) {
				data := &StringValueBase64{
					Base64: "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/wcAAwAB/1J8qkwAAAAASUVORK5CYII=",
				}
				require.Equal(t, `base64: iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/wcAAwAB/1J8qkwAAAAASUVORK5CYII=
`, MustMarshalToYamlString(data))
			})
		})
	})
	t.Run("json", func(t *testing.T) {
		t.Run("roundtrip", func(t *testing.T) {
			tests := []struct {
				name     string
				data     string
				expected StringValueType
			}{
				{
					name: "inline direct value",
					expected: &StringValueDirect{
						Value:          "https://example.com/some.png",
						IsDirectString: true,
					},
					data: `"https://example.com/some.png"`,
				},
				{
					name: "direct value",
					expected: &StringValueDirect{
						Value:          "https://example.com/some.png",
						IsDirectString: false,
					},
					data: `{"value":"https://example.com/some.png"}`,
				},
				{
					name: "base64",
					expected: &StringValueBase64{
						Base64: "ywAAAAAAQABAAACAUwAOw==",
					},
					data: `{"base64":"ywAAAAAAQABAAACAUwAOw=="}`,
				},
			}

			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					var val StringValue
					err := json.Unmarshal([]byte(test.data), &val)
					require.NoError(t, err)
					require.Equal(t, test.expected, val.Inner())
					require.Equal(t, strings.TrimSpace(test.data), strings.TrimSpace(MustMarshalToJsonString(val.Inner())))
				})
			}
		})
	})
}
