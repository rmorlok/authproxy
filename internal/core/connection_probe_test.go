package core

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/rmorlok/authproxy/internal/encfield"
	encryptmock "github.com/rmorlok/authproxy/internal/encrypt/mock"
	"github.com/rmorlok/authproxy/internal/schema/common"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnection_Probe(t *testing.T) {
	noProbes := newTestConnection(cschema.ConnectorDefinition{})
	hasProbes := newTestConnection(cschema.ConnectorDefinition{
		Probes: []cschema.Probe{
			{
				Id:     "probe-1",
				Period: common.HumanDurationFor("30m"),
			},
			{
				Id:     "probe-2",
				Period: common.HumanDurationFor("90m"),
			},
		},
	})

	t.Run("get probe", func(t *testing.T) {
		p, err := noProbes.GetProbe("does-not-exist")
		require.ErrorIs(t, err, ErrProbeNotFound)
		require.Nil(t, p)

		p, err = hasProbes.GetProbe("probe-2")
		require.NoError(t, err)
		require.Equal(t, "probe-2", p.GetId())

		p, err = hasProbes.GetProbe("does-not-exist")
		require.ErrorIs(t, err, ErrProbeNotFound)
		require.Nil(t, p)
	})
	t.Run("get probes", func(t *testing.T) {
		require.Empty(t, noProbes.GetProbes())

		probes := hasProbes.GetProbes()
		require.Len(t, probes, 2)
		require.Equal(t, "probe-1", probes[0].GetId())
		require.Equal(t, "probe-2", probes[1].GetId())
	})
}

func newConditionalProbeTestConnection(t *testing.T) *connection {
	t.Helper()

	conn := newTestConnection(cschema.ConnectorDefinition{
		Javascript: `
			function isEuRegion() {
				return cfg.region === "eu";
			}

			function isSalesforceConnection() {
				return labels["apxy/cxr/type"] === "salesforce";
			}
		`,
		Probes: []cschema.Probe{
			{
				Id: "cfg_true",
				If: &common.Predicate{Javascript: `isEuRegion()`},
			},
			{
				Id: "cfg_false",
				If: &common.Predicate{Javascript: `cfg.region === "us"`},
			},
			{
				Id: "label_true",
				If: &common.Predicate{Javascript: `isSalesforceConnection()`},
			},
			{
				Id: "annotation_true",
				If: &common.Predicate{Javascript: `annotations["setup-mode"] === "advanced"`},
			},
			{
				Id: "always",
			},
		},
	})
	setConnectionConfigFixture(t, conn, map[string]any{"region": "eu"})
	conn.Labels = map[string]string{"apxy/cxr/type": "salesforce"}
	conn.Annotations = map[string]string{"setup-mode": "advanced"}
	return conn
}

func TestConnection_GetEnabledProbes(t *testing.T) {
	conn := newConditionalProbeTestConnection(t)

	probes, err := conn.GetEnabledProbes(context.Background())
	require.NoError(t, err)
	require.Len(t, probes, 4)
	assert.Equal(t, "cfg_true", probes[0].GetId())
	assert.Equal(t, "label_true", probes[1].GetId())
	assert.Equal(t, "annotation_true", probes[2].GetId())
	assert.Equal(t, "always", probes[3].GetId())
}

func TestConnection_GetEnabledProbe(t *testing.T) {
	conn := newConditionalProbeTestConnection(t)

	p, err := conn.GetEnabledProbe(context.Background(), "cfg_true")
	require.NoError(t, err)
	assert.Equal(t, "cfg_true", p.GetId())

	p, err = conn.GetEnabledProbe(context.Background(), "cfg_false")
	require.ErrorIs(t, err, ErrProbeDisabled)
	require.Nil(t, p)

	p, err = conn.GetEnabledProbe(context.Background(), "missing")
	require.ErrorIs(t, err, ErrProbeNotFound)
	require.Nil(t, p)
}

func TestConnection_GetEnabledProbePredicateError(t *testing.T) {
	conn := newTestConnection(cschema.ConnectorDefinition{
		Probes: []cschema.Probe{
			{
				Id: "broken",
				If: &common.Predicate{Javascript: `cfg.region ===`},
			},
		},
	})

	p, err := conn.GetEnabledProbe(context.Background(), "broken")
	require.Error(t, err)
	require.Nil(t, p)
	assert.Contains(t, err.Error(), `probe "broken" if.javascript`)
}

func TestConnection_GetEnabledProbesPredicateError(t *testing.T) {
	conn := newTestConnection(cschema.ConnectorDefinition{
		Probes: []cschema.Probe{
			{
				Id: "broken",
				If: &common.Predicate{Javascript: `cfg.region ===`},
			},
		},
	})

	_, err := conn.GetEnabledProbes(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), `probe "broken" if.javascript`)
}

// TestConnection_ProbePredicateSourceBudget rejects oversized source before the
// connector cache compiles the library, counting library and condition together.
func TestConnection_ProbePredicateSourceBudget(t *testing.T) {
	conn := newTestConnection(cschema.ConnectorDefinition{
		Javascript: strings.Repeat(" ", probePredicateLimits().MaxSourceBytes),
		Probes: []cschema.Probe{{
			Id: "large-source", If: &common.Predicate{Javascript: "true"},
		}},
	})

	// Even a library that compiles to nothing still consumes authored source bytes.
	probe, err := conn.GetEnabledProbe(context.Background(), "large-source")
	require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
	require.Nil(t, probe)
	require.False(t, conn.connector.jsLoaded, "oversized input must not reach library compilation")
}

// TestConnection_ProbePredicateBounds fails closed when either initialization,
// authored evaluation, or input copying exceeds this caller's finite policy.
func TestConnection_ProbePredicateBounds(t *testing.T) {
	for name, test := range map[string]struct {
		library   string
		condition string
		config    map[string]any
	}{
		"library loop":   {library: "for (;;) {}", condition: "true"},
		"condition loop": {condition: "for (;;) {}"},
		"input bytes": {
			condition: "true",
			config:    map[string]any{"padding": strings.Repeat("x", probePredicateLimits().MaxInputBytes)},
		},
	} {
		t.Run(name, func(t *testing.T) {
			conn := newTestConnection(cschema.ConnectorDefinition{
				Javascript: test.library,
				Probes: []cschema.Probe{{
					Id: "bounded", If: &common.Predicate{Javascript: test.condition},
				}},
			})
			conn.configLoaded, conn.configCache = true, test.config

			// An earlier policy expiration is distinct from the caller's longer deadline.
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			probe, err := conn.GetEnabledProbe(ctx, "bounded")
			require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
			require.Nil(t, probe)
			require.NoError(t, ctx.Err())
		})
	}
}

// TestConnection_ProbePredicateCancellation preserves caller cancellation both
// before library compilation and while authored JavaScript is still executing.
func TestConnection_ProbePredicateCancellation(t *testing.T) {
	t.Run("before compilation", func(t *testing.T) {
		conn := newTestConnection(cschema.ConnectorDefinition{
			Javascript: "const privateCompileDetail = ;",
			Probes: []cschema.Probe{{
				Id: "canceled", If: &common.Predicate{Javascript: "true"},
			}},
		})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// The caller's cancellation wins without compiling invalid private source.
		probe, err := conn.GetEnabledProbe(ctx, "canceled")
		require.ErrorIs(t, err, context.Canceled)
		require.Nil(t, probe)
		require.False(t, conn.connector.jsLoaded)
	})

	t.Run("during evaluation", func(t *testing.T) {
		conn := newTestConnection(cschema.ConnectorDefinition{
			Probes: []cschema.Probe{{
				Id: "canceled", If: &common.Predicate{Javascript: "for (;;) {}"},
			}},
		})
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		// The request deadline interrupts the loop before the one-second policy timeout.
		probe, err := conn.GetEnabledProbe(ctx, "canceled")
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Nil(t, probe)
	})
}

// TestConnection_ProbePredicatePrivateErrors keeps authored source and throw
// payloads out of returned errors while retaining probe identity for diagnosis.
func TestConnection_ProbePredicatePrivateErrors(t *testing.T) {
	for name, test := range map[string]struct {
		library   string
		condition string
		want      error
	}{
		"library syntax": {library: "const privateCompileDetail = ;", condition: "true", want: jsruntime.ErrInvalidSource},
		"library throw":  {library: `throw new Error("privateLibraryDetail")`, condition: "true", want: jsruntime.ErrEvaluation},
		"condition throw": {
			condition: `throw {message: "privateConditionDetail"};`, want: jsruntime.ErrEvaluation,
		},
	} {
		t.Run(name, func(t *testing.T) {
			conn := newTestConnection(cschema.ConnectorDefinition{
				Javascript: test.library,
				Probes: []cschema.Probe{{
					Id: "safe-error", If: &common.Predicate{Javascript: test.condition},
				}},
			})

			// All failure stages use fixed runtime classifications rather than JavaScript diagnostics.
			probe, err := conn.GetEnabledProbe(context.Background(), "safe-error")
			require.ErrorIs(t, err, test.want)
			require.Nil(t, probe)
			require.Contains(t, err.Error(), `probe "safe-error" if.javascript`)
			require.NotContains(t, err.Error(), "private")
		})
	}
}

// TestConnection_ProbePredicatePreservesConnectionData checks that mutating a
// predicate's native snapshots cannot change stored metadata or the next probe.
func TestConnection_ProbePredicatePreservesConnectionData(t *testing.T) {
	conn := newTestConnection(cschema.ConnectorDefinition{
		Javascript: `function configuredRegion() { return cfg.region; }`,
		Probes: []cschema.Probe{
			{Id: "mutate", If: &common.Predicate{Javascript: `
				cfg.region = "changed"; labels.team = "changed"; annotations.note = "changed"; true;
			`}},
			{Id: "original", If: &common.Predicate{Javascript: `
				configuredRegion() === "eu" && labels.team === "platform" && annotations.note === "original"
			`}},
		},
	})
	conn.configLoaded, conn.configCache = true, map[string]any{"region": "eu"}
	conn.Labels = map[string]string{"team": "platform"}
	conn.Annotations = map[string]string{"note": "original"}

	// Statement-list predicates and helper reuse stay compatible across fresh invocations.
	probes, err := conn.GetEnabledProbes(context.Background())
	require.NoError(t, err)
	require.Len(t, probes, 2)
	require.Equal(t, "eu", conn.configCache["region"])
	require.Equal(t, "platform", conn.Labels["team"])
	require.Equal(t, "original", conn.Annotations["note"])
}

// lookupEnabledProbe exercises both public lookup paths and direct condition
// evaluation, so a contextual predicate cannot be preceded by a background load.
func lookupEnabledProbe(ctx context.Context, conn *connection, probe *cschema.Probe, lookup string) (bool, error) {
	switch lookup {
	case "single probe":
		result, err := conn.GetEnabledProbe(ctx, probe.Id)
		return result != nil, err
	case "probe list":
		result, err := conn.GetEnabledProbes(ctx)
		return len(result) != 0, err
	default:
		return conn.isProbeEnabled(ctx, probe)
	}
}

// TestConnection_ProbeDefinitionLoadFailure distinguishes definition-load errors
// from invalid JavaScript and leaves failed loads retryable through every caller.
func TestConnection_ProbeDefinitionLoadFailure(t *testing.T) {
	for _, lookup := range []string{"single probe", "probe list", "condition"} {
		for _, failure := range []string{"decrypt", "decode", "canceled during decrypt", "deadline during decrypt"} {
			t.Run(lookup+"/"+failure, func(t *testing.T) {
				// Start with an uncached encrypted definition and already-loaded connection data.
				probe := cschema.Probe{Id: "conditional", If: &common.Predicate{Javascript: "probeEnabled()"}}
				definition := cschema.ConnectorDefinition{
					Javascript: "function probeEnabled() { return true; }",
					Probes:     []cschema.Probe{probe},
				}
				conn := newTestConnection(definition)
				conn.configLoaded = true
				encryption := encryptmock.NewMockE(gomock.NewController(t))
				conn.s.encrypt = encryption
				encoded, err := json.Marshal(definition)
				require.NoError(t, err)
				require.Nil(t, conn.connector.def)
				loadFailure := errors.New("transient definition decryption failure")
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if failure == "deadline during decrypt" {
					ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
					defer cancel()
				}
				encryption.EXPECT().DecryptString(ctx, conn.connector.EncryptedDefinition).
					DoAndReturn(func(loadCtx context.Context, _ encfield.EncryptedField) (string, error) {
						switch failure {
						case "decrypt":
							return "", loadFailure
						case "decode":
							return "{", nil
						case "canceled during decrypt":
							cancel()
							return string(encoded), nil
						default:
							<-loadCtx.Done()
							return "", loadCtx.Err()
						}
					})

				// Loading fails through ordinary errors, preserving infrastructure/cancellation identity.
				var enabled bool
				require.NotPanics(t, func() {
					enabled, err = lookupEnabledProbe(ctx, conn, &probe, lookup)
				})
				require.Error(t, err)
				require.False(t, enabled)
				require.Contains(t, err.Error(), "load connector definition")
				require.NotErrorIs(t, err, jsruntime.ErrInvalidSource)
				switch failure {
				case "decrypt":
					require.ErrorIs(t, err, loadFailure)
				case "decode":
					var syntaxError *json.SyntaxError
					require.ErrorAs(t, err, &syntaxError)
				case "canceled during decrypt":
					require.ErrorIs(t, err, context.Canceled)
				default:
					require.ErrorIs(t, err, context.DeadlineExceeded)
				}
				require.Nil(t, conn.connector.def)
				require.False(t, conn.connector.jsLoaded)

				// A later live call retries loading, evaluates the predicate, and then reuses the cache.
				encryption.EXPECT().DecryptString(context.Background(), conn.connector.EncryptedDefinition).
					Return(string(encoded), nil)
				for range 2 {
					enabled, err = lookupEnabledProbe(context.Background(), conn, &probe, lookup)
					require.NoError(t, err)
					require.True(t, enabled)
				}
			})
		}
	}
}

// TestConnection_ProbeDefinitionCanceledBeforeLoad skips decryption entirely when
// the request is already canceled, including before the public lookup finds its probe.
func TestConnection_ProbeDefinitionCanceledBeforeLoad(t *testing.T) {
	for _, lookup := range []string{"single probe", "probe list", "condition"} {
		t.Run(lookup, func(t *testing.T) {
			// No encryption expectation is installed: any attempted load would fail this test.
			probe := cschema.Probe{Id: "conditional", If: &common.Predicate{Javascript: "true"}}
			conn := newTestConnection(cschema.ConnectorDefinition{Probes: []cschema.Probe{probe}})
			conn.s.encrypt = encryptmock.NewMockE(gomock.NewController(t))
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			// Both public lookup and direct condition evaluation retain caller cancellation.
			enabled, err := lookupEnabledProbe(ctx, conn, &probe, lookup)
			require.ErrorIs(t, err, context.Canceled)
			require.False(t, enabled)
			require.Nil(t, conn.connector.def)
			require.False(t, conn.connector.jsLoaded)
		})
	}
}
