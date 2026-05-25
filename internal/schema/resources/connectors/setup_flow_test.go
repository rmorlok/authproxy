package connectors

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSetupFlowValidation(t *testing.T) {
	vc := &common.ValidationContext{Path: "setup_flow"}

	t.Run("nil setup flow is valid", func(t *testing.T) {
		var sf *SetupFlow
		assert.NoError(t, sf.Validate(vc))
	})

	t.Run("empty setup flow is valid", func(t *testing.T) {
		sf := &SetupFlow{}
		assert.NoError(t, sf.Validate(vc))
	})

	t.Run("valid preconnect only", func(t *testing.T) {
		sf := &SetupFlow{
			Preconnect: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "tenant",
						Title:      "Enter tenant",
						JsonSchema: common.RawJSON(`{"type":"object","properties":{"tenant":{"type":"string"}}}`),
					},
				},
			},
		}
		assert.NoError(t, sf.Validate(vc))
	})

	t.Run("valid configure only", func(t *testing.T) {
		sf := &SetupFlow{
			Configure: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "workspace",
						JsonSchema: common.RawJSON(`{"type":"object"}`),
						DataSources: map[string]DataSourceDef{
							"workspaces": {
								ProxyRequest: &DataSourceProxyRequest{
									Method: "GET",
									Url:    "https://api.example.com/workspaces",
								},
								Transform: "data.map(w => ({value: w.id, label: w.name}))",
							},
						},
					},
				},
			},
		}
		assert.NoError(t, sf.Validate(vc))
	})

	t.Run("valid preconnect and configure", func(t *testing.T) {
		sf := &SetupFlow{
			Preconnect: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "tenant",
						JsonSchema: common.RawJSON(`{"type":"object"}`),
					},
				},
			},
			Configure: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "workspace",
						JsonSchema: common.RawJSON(`{"type":"object"}`),
					},
				},
			},
		}
		assert.NoError(t, sf.Validate(vc))
	})

	t.Run("duplicate step id across phases", func(t *testing.T) {
		sf := &SetupFlow{
			Preconnect: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "same-id",
						JsonSchema: common.RawJSON(`{"type":"object"}`),
					},
				},
			},
			Configure: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "same-id",
						JsonSchema: common.RawJSON(`{"type":"object"}`),
					},
				},
			},
		}
		err := sf.Validate(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate step id")
	})

	t.Run("duplicate step id within phase", func(t *testing.T) {
		sf := &SetupFlow{
			Configure: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "step1",
						JsonSchema: common.RawJSON(`{"type":"object"}`),
					},
					{
						Id:         "step1",
						JsonSchema: common.RawJSON(`{"type":"object"}`),
					},
				},
			},
		}
		err := sf.Validate(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate step id")
	})

	t.Run("empty phase has error", func(t *testing.T) {
		sf := &SetupFlow{
			Preconnect: &SetupFlowPhase{
				Steps: []SetupFlowStep{},
			},
		}
		err := sf.Validate(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must have at least one step")
	})

	t.Run("step missing id", func(t *testing.T) {
		sf := &SetupFlow{
			Preconnect: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						JsonSchema: common.RawJSON(`{"type":"object"}`),
					},
				},
			},
		}
		err := sf.Validate(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "step id is required")
	})

	t.Run("step missing json_schema", func(t *testing.T) {
		sf := &SetupFlow{
			Preconnect: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id: "step1",
					},
				},
			},
		}
		err := sf.Validate(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "json_schema is required")
	})

	t.Run("step with invalid json_schema", func(t *testing.T) {
		sf := &SetupFlow{
			Preconnect: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "step1",
						JsonSchema: common.RawJSON(`{not valid json}`),
					},
				},
			},
		}
		err := sf.Validate(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "json_schema is not valid JSON")
	})

	t.Run("step with invalid ui_schema", func(t *testing.T) {
		sf := &SetupFlow{
			Preconnect: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "step1",
						JsonSchema: common.RawJSON(`{"type":"object"}`),
						UiSchema:   common.RawJSON(`{bad json`),
					},
				},
			},
		}
		err := sf.Validate(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ui_schema is not valid JSON")
	})

	t.Run("data_sources not allowed in preconnect", func(t *testing.T) {
		sf := &SetupFlow{
			Preconnect: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "step1",
						JsonSchema: common.RawJSON(`{"type":"object"}`),
						DataSources: map[string]DataSourceDef{
							"items": {
								ProxyRequest: &DataSourceProxyRequest{Method: "GET", Url: "https://api.example.com"},
								Transform:    "data",
							},
						},
					},
				},
			},
		}
		err := sf.Validate(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "data_sources are not allowed in preconnect")
	})

	t.Run("data_sources allowed in configure", func(t *testing.T) {
		sf := &SetupFlow{
			Configure: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "step1",
						JsonSchema: common.RawJSON(`{"type":"object"}`),
						DataSources: map[string]DataSourceDef{
							"items": {
								ProxyRequest: &DataSourceProxyRequest{Method: "GET", Url: "https://api.example.com"},
								Transform:    "data",
							},
						},
					},
				},
			},
		}
		assert.NoError(t, sf.Validate(vc))
	})

	t.Run("data source missing proxy_request", func(t *testing.T) {
		sf := &SetupFlow{
			Configure: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "step1",
						JsonSchema: common.RawJSON(`{"type":"object"}`),
						DataSources: map[string]DataSourceDef{
							"items": {
								Transform: "data",
							},
						},
					},
				},
			},
		}
		err := sf.Validate(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "proxy_request is required")
	})

	t.Run("data source missing transform", func(t *testing.T) {
		sf := &SetupFlow{
			Configure: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "step1",
						JsonSchema: common.RawJSON(`{"type":"object"}`),
						DataSources: map[string]DataSourceDef{
							"items": {
								ProxyRequest: &DataSourceProxyRequest{Method: "GET", Url: "https://api.example.com"},
							},
						},
					},
				},
			},
		}
		err := sf.Validate(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "transform expression is required")
	})

	t.Run("proxy request missing method", func(t *testing.T) {
		sf := &SetupFlow{
			Configure: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "step1",
						JsonSchema: common.RawJSON(`{"type":"object"}`),
						DataSources: map[string]DataSourceDef{
							"items": {
								ProxyRequest: &DataSourceProxyRequest{Url: "https://api.example.com"},
								Transform:    "data",
							},
						},
					},
				},
			},
		}
		err := sf.Validate(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "method is required")
	})

	t.Run("proxy request missing url", func(t *testing.T) {
		sf := &SetupFlow{
			Configure: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "step1",
						JsonSchema: common.RawJSON(`{"type":"object"}`),
						DataSources: map[string]DataSourceDef{
							"items": {
								ProxyRequest: &DataSourceProxyRequest{Method: "GET"},
								Transform:    "data",
							},
						},
					},
				},
			},
		}
		err := sf.Validate(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "url is required")
	})
}

func TestSetupFlowHelpers(t *testing.T) {
	t.Run("HasPreconnect", func(t *testing.T) {
		assert.False(t, (*SetupFlow)(nil).HasPreconnect())
		assert.False(t, (&SetupFlow{}).HasPreconnect())
		assert.False(t, (&SetupFlow{Preconnect: &SetupFlowPhase{}}).HasPreconnect())
		assert.True(t, (&SetupFlow{Preconnect: &SetupFlowPhase{Steps: []SetupFlowStep{{Id: "s"}}}}).HasPreconnect())
	})

	t.Run("HasConfigure", func(t *testing.T) {
		assert.False(t, (*SetupFlow)(nil).HasConfigure())
		assert.False(t, (&SetupFlow{}).HasConfigure())
		assert.False(t, (&SetupFlow{Configure: &SetupFlowPhase{}}).HasConfigure())
		assert.True(t, (&SetupFlow{Configure: &SetupFlowPhase{Steps: []SetupFlowStep{{Id: "s"}}}}).HasConfigure())
	})
}

func TestParseSetupStep(t *testing.T) {
	t.Run("valid preconnect", func(t *testing.T) {
		s, err := ParseSetupStep("preconnect:0")
		require.NoError(t, err)
		assert.Equal(t, SetupPhasePreconnect, s.Phase())
		assert.Equal(t, 0, s.Index())
	})

	t.Run("valid configure with higher index", func(t *testing.T) {
		s, err := ParseSetupStep("configure:3")
		require.NoError(t, err)
		assert.Equal(t, SetupPhaseConfigure, s.Phase())
		assert.Equal(t, 3, s.Index())
	})

	t.Run("auth phase", func(t *testing.T) {
		s, err := ParseSetupStep("auth")
		require.NoError(t, err)
		assert.Equal(t, SetupPhaseAuth, s.Phase())
		assert.Equal(t, 0, s.Index())
		assert.True(t, s.Equals(SetupStepAuth))
	})

	t.Run("verify phase", func(t *testing.T) {
		s, err := ParseSetupStep("verify")
		require.NoError(t, err)
		assert.True(t, s.Equals(SetupStepVerify))
	})

	t.Run("verify_failed phase", func(t *testing.T) {
		s, err := ParseSetupStep("verify_failed")
		require.NoError(t, err)
		assert.True(t, s.Equals(SetupStepVerifyFailed))
		assert.True(t, s.IsTerminalFailure())
	})

	t.Run("auth_failed phase", func(t *testing.T) {
		s, err := ParseSetupStep("auth_failed")
		require.NoError(t, err)
		assert.True(t, s.Equals(SetupStepAuthFailed))
		assert.True(t, s.IsTerminalFailure())
	})

	t.Run("invalid format", func(t *testing.T) {
		_, err := ParseSetupStep("bad")
		assert.Error(t, err)
	})

	t.Run("invalid phase", func(t *testing.T) {
		_, err := ParseSetupStep("unknown:0")
		assert.Error(t, err)
	})

	t.Run("invalid index", func(t *testing.T) {
		_, err := ParseSetupStep("preconnect:abc")
		assert.Error(t, err)
	})

	t.Run("negative index", func(t *testing.T) {
		_, err := ParseSetupStep("preconnect:-1")
		assert.Error(t, err)
	})

	t.Run("singleton with index suffix is rejected", func(t *testing.T) {
		_, err := ParseSetupStep("auth:0")
		assert.Error(t, err)
	})
}

func TestSetupStepRoundTrip(t *testing.T) {
	cases := []string{"preconnect:0", "preconnect:7", "configure:0", "configure:2", "auth", "verify", "verify_failed", "auth_failed"}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			s, err := ParseSetupStep(c)
			require.NoError(t, err)
			assert.Equal(t, c, s.String())
		})
	}
}

func TestSetupStepConstructors(t *testing.T) {
	t.Run("NewSetupStep accepts singleton phases", func(t *testing.T) {
		s, err := NewSetupStep(SetupPhaseAuth)
		require.NoError(t, err)
		assert.Equal(t, "auth", s.String())
	})

	t.Run("NewSetupStep rejects indexed phase", func(t *testing.T) {
		_, err := NewSetupStep(SetupPhasePreconnect)
		assert.Error(t, err)
	})

	t.Run("NewSetupStep rejects unknown phase", func(t *testing.T) {
		_, err := NewSetupStep(SetupStepPhase("nope"))
		assert.Error(t, err)
	})

	t.Run("NewIndexedSetupStep accepts indexed phase", func(t *testing.T) {
		s, err := NewIndexedSetupStep(SetupPhaseConfigure, 4)
		require.NoError(t, err)
		assert.Equal(t, "configure:4", s.String())
	})

	t.Run("NewIndexedSetupStep rejects singleton phase", func(t *testing.T) {
		_, err := NewIndexedSetupStep(SetupPhaseAuth, 0)
		assert.Error(t, err)
	})

	t.Run("NewIndexedSetupStep rejects negative index", func(t *testing.T) {
		_, err := NewIndexedSetupStep(SetupPhasePreconnect, -1)
		assert.Error(t, err)
	})
}

func TestMustNewIndexedSetupStep(t *testing.T) {
	t.Run("returns step for valid indexed phase", func(t *testing.T) {
		s := MustNewIndexedSetupStep(SetupPhaseConfigure, 0)
		assert.Equal(t, SetupPhaseConfigure, s.Phase())
		assert.Equal(t, 0, s.Index())
		assert.Equal(t, "configure:0", s.String())
	})

	t.Run("returns step for preconnect with non-zero index", func(t *testing.T) {
		s := MustNewIndexedSetupStep(SetupPhasePreconnect, 3)
		assert.Equal(t, SetupPhasePreconnect, s.Phase())
		assert.Equal(t, 3, s.Index())
		assert.Equal(t, "preconnect:3", s.String())
	})

	t.Run("panics on singleton phase", func(t *testing.T) {
		assert.Panics(t, func() {
			MustNewIndexedSetupStep(SetupPhaseAuth, 0)
		})
	})

	t.Run("panics on negative index", func(t *testing.T) {
		assert.Panics(t, func() {
			MustNewIndexedSetupStep(SetupPhasePreconnect, -1)
		})
	})

	t.Run("panics on unknown phase", func(t *testing.T) {
		assert.Panics(t, func() {
			MustNewIndexedSetupStep(SetupStepPhase("nope"), 0)
		})
	})
}

func TestSetupStepZero(t *testing.T) {
	var z SetupStep
	assert.True(t, z.IsZero())
	assert.Equal(t, "", z.String())
}

func TestSetupStepJSONMarshal(t *testing.T) {
	t.Run("indexed phase marshals as canonical string", func(t *testing.T) {
		s := MustNewIndexedSetupStep(SetupPhaseConfigure, 2)
		b, err := json.Marshal(s)
		require.NoError(t, err)
		assert.Equal(t, `"configure:2"`, string(b))
	})

	t.Run("singleton phase marshals as canonical string", func(t *testing.T) {
		b, err := json.Marshal(SetupStepAuth)
		require.NoError(t, err)
		assert.Equal(t, `"auth"`, string(b))
	})

	t.Run("zero value marshals as null", func(t *testing.T) {
		var z SetupStep
		b, err := json.Marshal(z)
		require.NoError(t, err)
		assert.Equal(t, `null`, string(b))
	})

	t.Run("pointer to zero with omitempty is omitted", func(t *testing.T) {
		type wrapper struct {
			Step *SetupStep `json:"step,omitempty"`
		}
		b, err := json.Marshal(wrapper{Step: nil})
		require.NoError(t, err)
		assert.Equal(t, `{}`, string(b))
	})

	t.Run("pointer to non-zero marshals as string", func(t *testing.T) {
		type wrapper struct {
			Step *SetupStep `json:"step,omitempty"`
		}
		s := MustNewIndexedSetupStep(SetupPhasePreconnect, 0)
		b, err := json.Marshal(wrapper{Step: &s})
		require.NoError(t, err)
		assert.Equal(t, `{"step":"preconnect:0"}`, string(b))
	})
}

func TestSetupStepJSONUnmarshal(t *testing.T) {
	cases := []string{"preconnect:0", "preconnect:7", "configure:0", "configure:2", "auth", "verify", "verify_failed", "auth_failed"}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			input := []byte(`"` + c + `"`)
			var s SetupStep
			require.NoError(t, json.Unmarshal(input, &s))
			assert.Equal(t, c, s.String())

			// And round-trip
			b, err := json.Marshal(s)
			require.NoError(t, err)
			assert.Equal(t, string(input), string(b))
		})
	}

	t.Run("null produces zero", func(t *testing.T) {
		var s SetupStep
		require.NoError(t, json.Unmarshal([]byte(`null`), &s))
		assert.True(t, s.IsZero())
	})

	t.Run("empty string produces zero", func(t *testing.T) {
		var s SetupStep
		require.NoError(t, json.Unmarshal([]byte(`""`), &s))
		assert.True(t, s.IsZero())
	})

	t.Run("invalid string returns error", func(t *testing.T) {
		var s SetupStep
		err := json.Unmarshal([]byte(`"unknown:0"`), &s)
		assert.Error(t, err)
	})

	t.Run("non-string value returns error", func(t *testing.T) {
		var s SetupStep
		err := json.Unmarshal([]byte(`123`), &s)
		assert.Error(t, err)
	})
}

func TestSetupStepYAMLRoundTrip(t *testing.T) {
	cases := []string{"preconnect:0", "configure:2", "auth", "verify_failed"}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			parsed, err := ParseSetupStep(c)
			require.NoError(t, err)

			out, err := yaml.Marshal(parsed)
			require.NoError(t, err)

			var back SetupStep
			require.NoError(t, yaml.Unmarshal(out, &back))
			assert.Equal(t, c, back.String())
		})
	}

	t.Run("null yaml decodes to zero", func(t *testing.T) {
		var s SetupStep
		require.NoError(t, yaml.Unmarshal([]byte(`null`), &s))
		assert.True(t, s.IsZero())
	})

	t.Run("empty string yaml decodes to zero", func(t *testing.T) {
		var s SetupStep
		require.NoError(t, yaml.Unmarshal([]byte(`""`), &s))
		assert.True(t, s.IsZero())
	})

	t.Run("invalid string returns error", func(t *testing.T) {
		var s SetupStep
		err := yaml.Unmarshal([]byte(`unknown:0`), &s)
		assert.Error(t, err)
	})
}

func TestSetupStepPhaseHelpers(t *testing.T) {
	assert.True(t, SetupPhasePreconnect.IsIndexed())
	assert.True(t, SetupPhaseConfigure.IsIndexed())
	assert.False(t, SetupPhaseAuth.IsIndexed())
	assert.False(t, SetupPhaseVerify.IsIndexed())

	assert.True(t, SetupPhaseVerifyFailed.IsTerminalFailure())
	assert.True(t, SetupPhaseAuthFailed.IsTerminalFailure())
	assert.False(t, SetupPhaseAuth.IsTerminalFailure())
	assert.False(t, SetupPhasePreconnect.IsTerminalFailure())

	assert.False(t, SetupStepPhase("garbage").IsValid())
	assert.True(t, SetupPhaseAuth.IsValid())
}

func TestSetupFlowTotalSteps(t *testing.T) {
	assert.Equal(t, 0, (*SetupFlow)(nil).TotalSteps())
	assert.Equal(t, 0, (&SetupFlow{}).TotalSteps())

	sf := &SetupFlow{
		Preconnect: &SetupFlowPhase{
			Steps: []SetupFlowStep{{Id: "a"}, {Id: "b"}},
		},
		Configure: &SetupFlowPhase{
			Steps: []SetupFlowStep{{Id: "c"}},
		},
	}
	assert.Equal(t, 3, sf.TotalSteps())
}

func TestSetupFlowFirstSetupStep(t *testing.T) {
	assert.True(t, (*SetupFlow)(nil).FirstSetupStep().IsZero())

	t.Run("preconnect first", func(t *testing.T) {
		sf := &SetupFlow{
			Preconnect: &SetupFlowPhase{Steps: []SetupFlowStep{{Id: "a"}}},
			Configure:  &SetupFlowPhase{Steps: []SetupFlowStep{{Id: "b"}}},
		}
		assert.Equal(t, "preconnect:0", sf.FirstSetupStep().String())
	})

	t.Run("configure only", func(t *testing.T) {
		sf := &SetupFlow{
			Configure: &SetupFlowPhase{Steps: []SetupFlowStep{{Id: "a"}}},
		}
		assert.Equal(t, "configure:0", sf.FirstSetupStep().String())
	})
}

func TestSetupFlowNextSetupStep(t *testing.T) {
	sf := &SetupFlow{
		Preconnect: &SetupFlowPhase{
			Steps: []SetupFlowStep{{Id: "a"}, {Id: "b"}},
		},
		Configure: &SetupFlowPhase{
			Steps: []SetupFlowStep{{Id: "c"}, {Id: "d"}},
		},
	}

	mustParse := func(t *testing.T, s string) SetupStep {
		t.Helper()
		v, err := ParseSetupStep(s)
		require.NoError(t, err)
		return v
	}

	t.Run("preconnect:0 -> preconnect:1", func(t *testing.T) {
		next, err := sf.NextSetupStep(mustParse(t, "preconnect:0"), false)
		require.NoError(t, err)
		assert.Equal(t, "preconnect:1", next.String())
	})

	t.Run("preconnect:1 -> auth", func(t *testing.T) {
		next, err := sf.NextSetupStep(mustParse(t, "preconnect:1"), false)
		require.NoError(t, err)
		assert.True(t, next.Equals(SetupStepAuth))
	})

	t.Run("auth -> configure:0", func(t *testing.T) {
		next, err := sf.NextSetupStep(SetupStepAuth, false)
		require.NoError(t, err)
		assert.Equal(t, "configure:0", next.String())
	})

	t.Run("auth -> verify when probes present", func(t *testing.T) {
		next, err := sf.NextSetupStep(SetupStepAuth, true)
		require.NoError(t, err)
		assert.True(t, next.Equals(SetupStepVerify))
	})

	t.Run("verify -> configure:0", func(t *testing.T) {
		next, err := sf.NextSetupStep(SetupStepVerify, true)
		require.NoError(t, err)
		assert.Equal(t, "configure:0", next.String())
	})

	t.Run("verify with no configure -> zero (complete)", func(t *testing.T) {
		sfNoConfig := &SetupFlow{
			Preconnect: &SetupFlowPhase{Steps: []SetupFlowStep{{Id: "a"}}},
		}
		next, err := sfNoConfig.NextSetupStep(SetupStepVerify, true)
		require.NoError(t, err)
		assert.True(t, next.IsZero())
	})

	t.Run("configure:0 -> configure:1", func(t *testing.T) {
		next, err := sf.NextSetupStep(mustParse(t, "configure:0"), false)
		require.NoError(t, err)
		assert.Equal(t, "configure:1", next.String())
	})

	t.Run("configure:1 -> zero (complete)", func(t *testing.T) {
		next, err := sf.NextSetupStep(mustParse(t, "configure:1"), false)
		require.NoError(t, err)
		assert.True(t, next.IsZero())
	})

	t.Run("auth with no configure -> zero (complete)", func(t *testing.T) {
		sfNoConfig := &SetupFlow{
			Preconnect: &SetupFlowPhase{Steps: []SetupFlowStep{{Id: "a"}}},
		}
		next, err := sfNoConfig.NextSetupStep(SetupStepAuth, false)
		require.NoError(t, err)
		assert.True(t, next.IsZero())
	})
}

func TestSetupFlowGetStepBySetupStep(t *testing.T) {
	sf := &SetupFlow{
		Preconnect: &SetupFlowPhase{
			Steps: []SetupFlowStep{{Id: "tenant"}, {Id: "region"}},
		},
		Configure: &SetupFlowPhase{
			Steps: []SetupFlowStep{{Id: "workspace"}},
		},
	}

	mustParse := func(t *testing.T, s string) SetupStep {
		t.Helper()
		v, err := ParseSetupStep(s)
		require.NoError(t, err)
		return v
	}

	t.Run("preconnect:0", func(t *testing.T) {
		step, idx, err := sf.GetStepBySetupStep(mustParse(t, "preconnect:0"))
		require.NoError(t, err)
		assert.Equal(t, "tenant", step.Id)
		assert.Equal(t, 0, idx)
	})

	t.Run("preconnect:1", func(t *testing.T) {
		step, idx, err := sf.GetStepBySetupStep(mustParse(t, "preconnect:1"))
		require.NoError(t, err)
		assert.Equal(t, "region", step.Id)
		assert.Equal(t, 1, idx)
	})

	t.Run("configure:0 global index includes preconnect", func(t *testing.T) {
		step, idx, err := sf.GetStepBySetupStep(mustParse(t, "configure:0"))
		require.NoError(t, err)
		assert.Equal(t, "workspace", step.Id)
		assert.Equal(t, 2, idx) // 2 preconnect steps before this
	})

	t.Run("out of range", func(t *testing.T) {
		_, _, err := sf.GetStepBySetupStep(mustParse(t, "preconnect:5"))
		assert.Error(t, err)
	})

	t.Run("rejects singleton phase", func(t *testing.T) {
		_, _, err := sf.GetStepBySetupStep(SetupStepAuth)
		assert.Error(t, err)
	})
}

func TestSetupFlowYAMLRoundtrip(t *testing.T) {
	t.Run("parse connector with setup_flow from YAML", func(t *testing.T) {
		data, err := os.ReadFile("test_data/valid-oauth-connector-with-setup-flow.yaml")
		require.NoError(t, err)

		var connector Connector
		err = yaml.Unmarshal(data, &connector)
		require.NoError(t, err)

		require.NotNil(t, connector.SetupFlow)

		// Preconnect
		require.True(t, connector.SetupFlow.HasPreconnect())
		require.Len(t, connector.SetupFlow.Preconnect.Steps, 1)
		assert.Equal(t, "tenant", connector.SetupFlow.Preconnect.Steps[0].Id)
		assert.Equal(t, "Enter your Acme tenant", connector.SetupFlow.Preconnect.Steps[0].Title)
		assert.NotEmpty(t, connector.SetupFlow.Preconnect.Steps[0].JsonSchema)
		assert.NotEmpty(t, connector.SetupFlow.Preconnect.Steps[0].UiSchema)
		assert.Empty(t, connector.SetupFlow.Preconnect.Steps[0].DataSources)

		// Configure
		require.True(t, connector.SetupFlow.HasConfigure())
		require.Len(t, connector.SetupFlow.Configure.Steps, 2)

		step1 := connector.SetupFlow.Configure.Steps[0]
		assert.Equal(t, "select_workspace", step1.Id)
		assert.Equal(t, "Select Workspace", step1.Title)
		require.Len(t, step1.DataSources, 1)
		ds := step1.DataSources["workspaces"]
		require.NotNil(t, ds.ProxyRequest)
		assert.Equal(t, "GET", ds.ProxyRequest.Method)
		assert.Equal(t, "https://api.acme.com/v1/workspaces", ds.ProxyRequest.Url)
		assert.NotEmpty(t, ds.Transform)

		step2 := connector.SetupFlow.Configure.Steps[1]
		assert.Equal(t, "sync_options", step2.Id)
		assert.Empty(t, step2.DataSources)

		// Validate
		vc := &common.ValidationContext{Path: "connector"}
		assert.NoError(t, connector.Validate(vc))
	})

	t.Run("json roundtrip preserves setup_flow", func(t *testing.T) {
		sf := &SetupFlow{
			Preconnect: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "tenant",
						Title:      "Enter tenant",
						JsonSchema: common.RawJSON(`{"type":"object","properties":{"tenant":{"type":"string"}}}`),
						UiSchema:   common.RawJSON(`{"type":"VerticalLayout"}`),
					},
				},
			},
			Configure: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "workspace",
						JsonSchema: common.RawJSON(`{"type":"object"}`),
						DataSources: map[string]DataSourceDef{
							"items": {
								ProxyRequest: &DataSourceProxyRequest{
									Method:  "GET",
									Url:     "https://api.example.com/items",
									Headers: map[string]string{"Accept": "application/json"},
								},
								Transform: "data.map(i => ({value: i.id, label: i.name}))",
							},
						},
					},
				},
			},
		}

		jsonBytes, err := json.Marshal(sf)
		require.NoError(t, err)

		var parsed SetupFlow
		err = json.Unmarshal(jsonBytes, &parsed)
		require.NoError(t, err)

		require.True(t, parsed.HasPreconnect())
		assert.Equal(t, "tenant", parsed.Preconnect.Steps[0].Id)
		require.True(t, parsed.HasConfigure())
		assert.Equal(t, "workspace", parsed.Configure.Steps[0].Id)
		require.NotNil(t, parsed.Configure.Steps[0].DataSources["items"].ProxyRequest)
		assert.Equal(t, "GET", parsed.Configure.Steps[0].DataSources["items"].ProxyRequest.Method)
		assert.Equal(t, "application/json", parsed.Configure.Steps[0].DataSources["items"].ProxyRequest.Headers["Accept"])
	})
}

func TestSetupFlowStepValidateAndMergeData(t *testing.T) {
	tenantSchema := common.RawJSON(`{
		"type": "object",
		"required": ["tenant"],
		"properties": {
			"tenant": {"type": "string"}
		}
	}`)

	t.Run("rejects empty step_id", func(t *testing.T) {
		step := &SetupFlowStep{Id: "tenant", JsonSchema: tenantSchema}

		_, err := step.ValidateAndMergeData("", json.RawMessage(`{"tenant":"acme"}`), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "step_id is required")
	})

	t.Run("rejects mismatched step_id", func(t *testing.T) {
		step := &SetupFlowStep{Id: "tenant", JsonSchema: tenantSchema}

		_, err := step.ValidateAndMergeData("wrong_id", json.RawMessage(`{"tenant":"acme"}`), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not match current step")
	})

	t.Run("rejects missing required field", func(t *testing.T) {
		step := &SetupFlowStep{Id: "tenant", JsonSchema: tenantSchema}

		_, err := step.ValidateAndMergeData("tenant", json.RawMessage(`{}`), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "validation failed")
	})

	t.Run("rejects wrong type for field", func(t *testing.T) {
		schema := common.RawJSON(`{
			"type": "object",
			"properties": {
				"count": {"type": "integer"}
			}
		}`)
		step := &SetupFlowStep{Id: "step1", JsonSchema: schema}

		_, err := step.ValidateAndMergeData("step1", json.RawMessage(`{"count": "not-a-number"}`), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "validation failed")
	})

	t.Run("rejects invalid JSON data", func(t *testing.T) {
		step := &SetupFlowStep{Id: "tenant", JsonSchema: tenantSchema}

		_, err := step.ValidateAndMergeData("tenant", json.RawMessage(`{bad json`), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid form data JSON")
	})

	t.Run("merges valid data into nil config", func(t *testing.T) {
		step := &SetupFlowStep{Id: "tenant", JsonSchema: tenantSchema}

		cfg, err := step.ValidateAndMergeData("tenant", json.RawMessage(`{"tenant":"acme"}`), nil)
		require.NoError(t, err)
		assert.Equal(t, "acme", cfg["tenant"])
	})

	t.Run("merges valid data into existing config", func(t *testing.T) {
		step := &SetupFlowStep{Id: "tenant", JsonSchema: tenantSchema}
		existing := map[string]any{"region": "us-east"}

		cfg, err := step.ValidateAndMergeData("tenant", json.RawMessage(`{"tenant":"acme"}`), existing)
		require.NoError(t, err)
		assert.Equal(t, "acme", cfg["tenant"])
		assert.Equal(t, "us-east", cfg["region"])
	})

	t.Run("strips fields not in schema properties", func(t *testing.T) {
		step := &SetupFlowStep{Id: "tenant", JsonSchema: tenantSchema}

		cfg, err := step.ValidateAndMergeData("tenant", json.RawMessage(`{"tenant":"acme","injected_secret":"evil","admin":true}`), nil)
		require.NoError(t, err)
		assert.Equal(t, "acme", cfg["tenant"])
		assert.NotContains(t, cfg, "injected_secret")
		assert.NotContains(t, cfg, "admin")
	})

	t.Run("extra fields cannot overwrite prior config values", func(t *testing.T) {
		schema := common.RawJSON(`{
			"type": "object",
			"properties": {
				"workspace": {"type": "string"}
			}
		}`)
		step := &SetupFlowStep{Id: "step2", JsonSchema: schema}

		existing := map[string]any{"tenant": "acme"}
		cfg, err := step.ValidateAndMergeData("step2", json.RawMessage(`{"workspace":"main","tenant":"evil-override"}`), existing)
		require.NoError(t, err)
		assert.Equal(t, "acme", cfg["tenant"], "tenant should not be overwritten")
		assert.Equal(t, "main", cfg["workspace"])
	})

	t.Run("strips prototype pollution fields", func(t *testing.T) {
		schema := common.RawJSON(`{
			"type": "object",
			"properties": {
				"name": {"type": "string"}
			}
		}`)
		step := &SetupFlowStep{Id: "step1", JsonSchema: schema}

		cfg, err := step.ValidateAndMergeData("step1", json.RawMessage(`{"name":"legit","__proto__":"attack","constructor":"evil"}`), nil)
		require.NoError(t, err)
		assert.Equal(t, "legit", cfg["name"])
		assert.NotContains(t, cfg, "__proto__")
		assert.NotContains(t, cfg, "constructor")
	})
}
