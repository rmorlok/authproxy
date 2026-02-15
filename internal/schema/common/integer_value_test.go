package common

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestIntegerValue(t *testing.T) {
	t.Run("round trip starting from objects", func(t *testing.T) {
		tests := []struct {
			name string
			Val  IntegerValueType
		}{
			{
				name: "inline direct value",
				Val: &IntegerValueDirect{
					Value:    42,
					IsDirect: true,
				},
			},
			{
				name: "inline direct",
				Val: &IntegerValueDirect{
					Value:    42,
					IsDirect: false,
				},
			},
			{
				name: "env var",
				Val: &IntegerValueEnvVar{
					EnvVar: "SOME_ENV_VAR",
				},
			},
			{
				name: "env var - default",
				Val: &IntegerValueEnvVar{
					EnvVar:  "SOME_ENV_VAR",
					Default: util.ToPtr(int64(42)),
				},
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				t.Run("yaml", func(t *testing.T) {
					wrapper := &IntegerValue{test.Val}
					data, err := yaml.Marshal(wrapper)
					require.NoError(t, err)

					var output IntegerValue
					err = yaml.Unmarshal(data, &output)
					require.NoError(t, err)
					require.Equal(t, test.Val, output.Inner())
				})
				t.Run("json", func(t *testing.T) {
					wrapper := &IntegerValue{test.Val}
					data, err := json.Marshal(wrapper)
					require.NoError(t, err)

					var output IntegerValue
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
				expected IntegerValueType
			}{
				{
					name: "inline direct value",
					expected: &IntegerValueDirect{
						Value:    8080,
						IsDirect: true,
					},
					data: `8080`,
				},
				{
					name: "direct value",
					expected: &IntegerValueDirect{
						Value:    8080,
						IsDirect: false,
					},
					data: `
value: 8080
`,
				},
			}

			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					var val IntegerValue
					err := yaml.Unmarshal([]byte(test.data), &val)
					require.NoError(t, err)
					require.Equal(t, test.expected, val.Inner())
					require.Equal(t, strings.TrimSpace(test.data), strings.TrimSpace(MustMarshalToYamlString(&val)))
				})
			}
		})
		t.Run("parse", func(t *testing.T) {
			t.Run("inline direct value", func(t *testing.T) {
				data := `42
`
				var val IntegerValue
				err := yaml.Unmarshal([]byte(data), &val)
				require.NoError(t, err)
				require.Equal(t, &IntegerValueDirect{
					Value:    42,
					IsDirect: true,
				}, val.Inner())
			})
			t.Run("direct value", func(t *testing.T) {
				data := `
value: 8080
`
				var val IntegerValue
				err := yaml.Unmarshal([]byte(data), &val)
				require.NoError(t, err)
				require.Equal(t, &IntegerValueDirect{
					Value:    8080,
					IsDirect: false,
				}, val.Inner())
			})
		})

		t.Run("yaml gen", func(t *testing.T) {
			t.Run("inline value direct", func(t *testing.T) {
				data := &IntegerValueDirect{
					Value:    1955,
					IsDirect: true,
				}
				require.Equal(t, "1955\n", MustMarshalToYamlString(data))
			})
			t.Run("value direct", func(t *testing.T) {
				data := &IntegerValueDirect{
					Value: 8080,
				}
				require.Equal(t, "value: 8080\n", MustMarshalToYamlString(data))
			})
		})
	})
	t.Run("json", func(t *testing.T) {
		t.Run("roundtrip", func(t *testing.T) {
			tests := []struct {
				name     string
				data     string
				expected IntegerValueType
			}{
				{
					name: "inline direct value",
					expected: &IntegerValueDirect{
						Value:    8080,
						IsDirect: true,
					},
					data: `8080`,
				},
				{
					name: "direct value",
					expected: &IntegerValueDirect{
						Value:    8080,
						IsDirect: false,
					},
					data: `{"value":8080}`,
				},
			}

			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					var val IntegerValue
					err := json.Unmarshal([]byte(test.data), &val)
					require.NoError(t, err)
					require.Equal(t, test.expected, val.Inner())
					require.Equal(t, strings.TrimSpace(test.data), strings.TrimSpace(MustMarshalToJsonString(val.Inner())))
				})
			}
		})
		t.Run("parse", func(t *testing.T) {
			t.Run("inline direct value", func(t *testing.T) {
				data := `99`
				var val IntegerValue
				err := json.Unmarshal([]byte(data), &val)
				require.NoError(t, err)
				require.Equal(t, &IntegerValueDirect{
					Value:    99,
					IsDirect: true,
				}, val.Inner())
			})
			t.Run("direct value", func(t *testing.T) {
				data := `{"value": 99}`
				var val IntegerValue
				err := json.Unmarshal([]byte(data), &val)
				require.NoError(t, err)
				require.Equal(t, &IntegerValueDirect{
					Value:    99,
					IsDirect: false,
				}, val.Inner())
			})
		})
	})
}
