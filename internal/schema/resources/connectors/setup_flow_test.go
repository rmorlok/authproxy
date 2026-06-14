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

	t.Run("valid conditional step", func(t *testing.T) {
		sf := &SetupFlow{
			Configure: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "advanced_options",
						JsonSchema: common.RawJSON(`{"type":"object"}`),
						If: &SetupFlowStepIf{
							Javascript: `cfg.region === "eu" && labels["apxy/cxr/type"] === "salesforce"`,
						},
					},
				},
			},
		}
		assert.NoError(t, sf.Validate(vc))
	})

	t.Run("conditional step missing javascript", func(t *testing.T) {
		sf := &SetupFlow{
			Configure: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "advanced_options",
						JsonSchema: common.RawJSON(`{"type":"object"}`),
						If:         &SetupFlowStepIf{},
					},
				},
			},
		}
		err := sf.Validate(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "javascript is required")
	})

	t.Run("conditional step rejects blank javascript", func(t *testing.T) {
		sf := &SetupFlow{
			Configure: &SetupFlowPhase{
				Steps: []SetupFlowStep{
					{
						Id:         "advanced_options",
						JsonSchema: common.RawJSON(`{"type":"object"}`),
						If:         &SetupFlowStepIf{Javascript: " \n\t "},
					},
				},
			},
		}
		err := sf.Validate(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "javascript is required")
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
	t.Run("user-authored id", func(t *testing.T) {
		s, err := ParseSetupStep("tenant")
		require.NoError(t, err)
		assert.Equal(t, "tenant", s.Id())
	})

	t.Run("apxy-emitted pseudo-step", func(t *testing.T) {
		s, err := ParseSetupStep("apxy:verify")
		require.NoError(t, err)
		assert.True(t, s.Equals(SetupStepVerify))
		assert.True(t, s.IsApxyEmitted())
	})

	t.Run("verify_failed terminal", func(t *testing.T) {
		s, err := ParseSetupStep("apxy:verify_failed")
		require.NoError(t, err)
		assert.True(t, s.Equals(SetupStepVerifyFailed))
		assert.True(t, s.IsTerminalFailure())
	})

	t.Run("auth_failed terminal", func(t *testing.T) {
		s, err := ParseSetupStep("apxy:auth_failed")
		require.NoError(t, err)
		assert.True(t, s.Equals(SetupStepAuthFailed))
		assert.True(t, s.IsTerminalFailure())
	})

	t.Run("empty string produces zero", func(t *testing.T) {
		s, err := ParseSetupStep("")
		require.NoError(t, err)
		assert.True(t, s.IsZero())
	})
}

func TestSetupStepRoundTrip(t *testing.T) {
	cases := []string{"tenant", "select_workspace", "apxy:auth", "apxy:verify", "apxy:verify_failed", "apxy:auth_failed", "apxy:auth:oauth2_authorize"}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			s, err := ParseSetupStep(c)
			require.NoError(t, err)
			assert.Equal(t, c, s.String())
			assert.Equal(t, c, s.Id())
		})
	}
}

func TestSetupStepConstructors(t *testing.T) {
	t.Run("NewSetupStep accepts user-authored id", func(t *testing.T) {
		s, err := NewSetupStep("tenant")
		require.NoError(t, err)
		assert.Equal(t, "tenant", s.String())
		assert.False(t, s.IsApxyEmitted())
	})

	t.Run("NewSetupStep accepts apxy-prefixed id", func(t *testing.T) {
		s, err := NewSetupStep("apxy:auth:oauth2_authorize")
		require.NoError(t, err)
		assert.Equal(t, "apxy:auth:oauth2_authorize", s.String())
		assert.True(t, s.IsApxyEmitted())
	})

	t.Run("NewSetupStep rejects empty id", func(t *testing.T) {
		_, err := NewSetupStep("")
		assert.Error(t, err)
	})

	t.Run("MustNewSetupStep panics on empty id", func(t *testing.T) {
		assert.Panics(t, func() { MustNewSetupStep("") })
	})
}

func TestSetupStepZero(t *testing.T) {
	var z SetupStep
	assert.True(t, z.IsZero())
	assert.Equal(t, "", z.String())
}

func TestSetupStepJSONMarshal(t *testing.T) {
	t.Run("user-authored id marshals as JSON string", func(t *testing.T) {
		s := MustNewSetupStep("tenant")
		b, err := json.Marshal(s)
		require.NoError(t, err)
		assert.Equal(t, `"tenant"`, string(b))
	})

	t.Run("apxy-emitted id marshals as JSON string", func(t *testing.T) {
		b, err := json.Marshal(SetupStepVerify)
		require.NoError(t, err)
		assert.Equal(t, `"apxy:verify"`, string(b))
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
		s := MustNewSetupStep("tenant")
		b, err := json.Marshal(wrapper{Step: &s})
		require.NoError(t, err)
		assert.Equal(t, `{"step":"tenant"}`, string(b))
	})
}

func TestSetupStepJSONUnmarshal(t *testing.T) {
	cases := []string{"tenant", "select_workspace", "apxy:auth", "apxy:verify", "apxy:verify_failed", "apxy:auth_failed"}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			input := []byte(`"` + c + `"`)
			var s SetupStep
			require.NoError(t, json.Unmarshal(input, &s))
			assert.Equal(t, c, s.String())

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

	t.Run("non-string value returns error", func(t *testing.T) {
		var s SetupStep
		err := json.Unmarshal([]byte(`123`), &s)
		assert.Error(t, err)
	})
}

func TestSetupStepYAMLRoundTrip(t *testing.T) {
	cases := []string{"tenant", "apxy:verify", "apxy:verify_failed"}
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
}

func TestSetupStepIsApxyEmitted(t *testing.T) {
	assert.True(t, SetupStepVerify.IsApxyEmitted())
	assert.True(t, SetupStepVerifyFailed.IsApxyEmitted())
	assert.True(t, SetupStepAuthFailed.IsApxyEmitted())
	// Auth-method-emitted steps (constructed in their own packages) also
	// carry the prefix; the schema only knows the convention.
	assert.True(t, MustNewSetupStep("apxy:auth:oauth2_authorize").IsApxyEmitted())
	assert.False(t, MustNewSetupStep("tenant").IsApxyEmitted())
}

func TestSetupStepIsTerminalFailure(t *testing.T) {
	assert.True(t, SetupStepVerifyFailed.IsTerminalFailure())
	assert.True(t, SetupStepAuthFailed.IsTerminalFailure())
	assert.False(t, SetupStepVerify.IsTerminalFailure())
	assert.False(t, MustNewSetupStep("apxy:auth:oauth2_authorize").IsTerminalFailure())
	assert.False(t, MustNewSetupStep("tenant").IsTerminalFailure())
}

// TestSetupFlowStep_RejectsApxyPrefix — user-authored step ids must not start
// with the reserved apxy: prefix (which is for system-emitted steps).
func TestSetupFlowStep_RejectsApxyPrefix(t *testing.T) {
	sf := &SetupFlow{
		Preconnect: &SetupFlowPhase{
			Steps: []SetupFlowStep{{
				Id:         "apxy:my_step",
				JsonSchema: common.RawJSON(`{"type":"object"}`),
			}},
		},
	}
	err := sf.Validate(&common.ValidationContext{Path: "setup_flow"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reserved prefix")
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

// TestSetupFlow_FindStepById covers the schema-only step lookup. The runtime
// ManifestSetupFlow (core/iface) handles auth-method-emitted + pseudo steps;
// this method only sees user-authored preconnect / configure steps.
func TestSetupFlow_FindStepById(t *testing.T) {
	sf := &SetupFlow{
		Preconnect: &SetupFlowPhase{Steps: []SetupFlowStep{{Id: "tenant"}, {Id: "region"}}},
		Configure:  &SetupFlowPhase{Steps: []SetupFlowStep{{Id: "workspace"}}},
	}

	t.Run("preconnect step", func(t *testing.T) {
		s, ok := sf.FindStepById("tenant")
		require.True(t, ok)
		assert.Equal(t, "tenant", s.Id)
	})

	t.Run("configure step", func(t *testing.T) {
		s, ok := sf.FindStepById("workspace")
		require.True(t, ok)
		assert.Equal(t, "workspace", s.Id)
	})

	t.Run("unknown id", func(t *testing.T) {
		_, ok := sf.FindStepById("nope")
		assert.False(t, ok)
	})

	t.Run("nil flow", func(t *testing.T) {
		_, ok := (*SetupFlow)(nil).FindStepById("x")
		assert.False(t, ok)
	})
}

func TestSetupFlow_IsConfigureStep(t *testing.T) {
	sf := &SetupFlow{
		Preconnect: &SetupFlowPhase{Steps: []SetupFlowStep{{Id: "tenant"}}},
		Configure:  &SetupFlowPhase{Steps: []SetupFlowStep{{Id: "workspace"}}},
	}

	assert.False(t, sf.IsConfigureStep(MustNewSetupStep("tenant")))
	assert.True(t, sf.IsConfigureStep(MustNewSetupStep("workspace")))
	assert.False(t, sf.IsConfigureStep(SetupStepVerify))
	assert.False(t, sf.IsConfigureStep(SetupStep{}))
}

// TestSetupFlowStep_RedirectValidation covers the per-type required/forbidden
// field rules. Form steps must declare json_schema; redirect steps must
// declare redirect.url and must not declare form-only fields.
func TestSetupFlowStep_RedirectValidation(t *testing.T) {
	vc := &common.ValidationContext{Path: "setup_flow"}

	t.Run("valid redirect step", func(t *testing.T) {
		sf := &SetupFlow{
			Preconnect: &SetupFlowPhase{Steps: []SetupFlowStep{{
				Id:       "bounce_to_partner",
				Type:     SetupFlowStepTypeRedirect,
				Redirect: &SetupFlowStepRedirect{URL: "https://partner.example.com/auth?return={{RETURN_ADVANCE}}"},
			}}},
		}
		assert.NoError(t, sf.Validate(vc))
	})

	t.Run("redirect step missing redirect block", func(t *testing.T) {
		sf := &SetupFlow{
			Preconnect: &SetupFlowPhase{Steps: []SetupFlowStep{{
				Id:   "bounce",
				Type: SetupFlowStepTypeRedirect,
			}}},
		}
		err := sf.Validate(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "redirect is required")
	})

	t.Run("redirect step missing url", func(t *testing.T) {
		sf := &SetupFlow{
			Preconnect: &SetupFlowPhase{Steps: []SetupFlowStep{{
				Id:       "bounce",
				Type:     SetupFlowStepTypeRedirect,
				Redirect: &SetupFlowStepRedirect{},
			}}},
		}
		err := sf.Validate(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "url is required")
	})

	t.Run("redirect step rejects form-only fields", func(t *testing.T) {
		sf := &SetupFlow{
			Preconnect: &SetupFlowPhase{Steps: []SetupFlowStep{{
				Id:         "bounce",
				Type:       SetupFlowStepTypeRedirect,
				Redirect:   &SetupFlowStepRedirect{URL: "https://x.example.com"},
				JsonSchema: common.RawJSON(`{"type":"object"}`),
				UiSchema:   common.RawJSON(`{"type":"VerticalLayout"}`),
			}}},
		}
		err := sf.Validate(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "json_schema must be absent for redirect steps")
		assert.Contains(t, err.Error(), "ui_schema must be absent for redirect steps")
	})

	t.Run("form step rejects redirect block", func(t *testing.T) {
		sf := &SetupFlow{
			Preconnect: &SetupFlowPhase{Steps: []SetupFlowStep{{
				Id:         "tenant",
				Type:       SetupFlowStepTypeForm,
				JsonSchema: common.RawJSON(`{"type":"object"}`),
				Redirect:   &SetupFlowStepRedirect{URL: "https://x"},
			}}},
		}
		err := sf.Validate(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "redirect must be absent for form steps")
	})

	t.Run("unknown type rejected", func(t *testing.T) {
		sf := &SetupFlow{
			Preconnect: &SetupFlowPhase{Steps: []SetupFlowStep{{
				Id:   "x",
				Type: "broken",
			}}},
		}
		err := sf.Validate(vc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `type must be`)
	})

	t.Run("empty type defaults to form", func(t *testing.T) {
		sf := &SetupFlow{
			Preconnect: &SetupFlowPhase{Steps: []SetupFlowStep{{
				Id:         "tenant",
				JsonSchema: common.RawJSON(`{"type":"object"}`),
			}}},
		}
		assert.NoError(t, sf.Validate(vc))
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
		require.NotNil(t, step2.If)
		assert.Equal(t, "cfg.region === \"eu\" && labels[\"apxy/cxr/type\"] === \"acme-crm\"\n", step2.If.Javascript)

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
						If: &SetupFlowStepIf{
							Javascript: `annotations["setup-mode"] !== "basic"`,
						},
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
		require.NotNil(t, parsed.Configure.Steps[0].If)
		assert.Equal(t, `annotations["setup-mode"] !== "basic"`, parsed.Configure.Steps[0].If.Javascript)
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
