package core

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apjs"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/rmorlok/authproxy/internal/core/iface"
	encryptmock "github.com/rmorlok/authproxy/internal/encrypt/mock"
	"github.com/rmorlok/authproxy/internal/schema/common"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManifestSetupFlowIfJavascript(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	sf := &cschema.SetupFlow{
		Configure: &cschema.SetupFlowPhase{
			Steps: []cschema.SetupFlowStep{
				{
					Id:         "cfg_false",
					JsonSchema: workspaceSchema,
					If:         &common.Predicate{Javascript: `cfg.region === "us"`},
				},
				{
					Id:         "label_true",
					JsonSchema: workspaceSchema,
					If:         &common.Predicate{Javascript: `isSalesforceConnection()`},
				},
				{
					Id:         "annotation_true",
					JsonSchema: workspaceSchema,
					If:         &common.Predicate{Javascript: `annotations["setup-mode"] === "advanced"`},
				},
			},
		},
	}
	conn, _ := newTestConnectionWithSetupFlow(t, ctrl, sf)
	conn.connector = NewTestConnector(cschema.ConnectorDefinition{
		Javascript: `
			function isSalesforceConnection() {
				return labels["apxy/cxr/type"] === "salesforce";
			}
		`,
		SetupFlow: sf,
	})
	setConnectionConfigFixture(t, conn, map[string]any{"region": "eu"})
	conn.Labels = map[string]string{"apxy/cxr/type": "salesforce"}
	conn.Annotations = map[string]string{"setup-mode": "advanced"}

	flow := conn.s.buildManifestSetupFlow(conn)
	steps, err := flow.Steps(context.Background())
	require.NoError(t, err)
	require.Len(t, steps, 2)
	assert.Equal(t, "label_true", steps[0].Id())
	assert.Equal(t, "annotation_true", steps[1].Id())

	first, err := flow.FirstStep(context.Background())
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, "label_true", first.Id())

	next, ok, err := flow.NextStep(context.Background(), "label_true")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "annotation_true", next.Id())
}

func TestManifestSetupFlowIfJavascriptVerifyBoundary(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn, _ := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{
		Preconnect: &cschema.SetupFlowPhase{
			Steps: []cschema.SetupFlowStep{
				{
					Id:         "us_only",
					JsonSchema: regionSchema,
					If:         &common.Predicate{Javascript: `cfg.region === "us"`},
				},
			},
		},
		Configure: &cschema.SetupFlowPhase{
			Steps: []cschema.SetupFlowStep{
				{Id: "workspace", JsonSchema: workspaceSchema},
			},
		},
	})
	setConnectionConfigFixture(t, conn, map[string]any{"region": "eu"})
	conn.connector.GetDefinition().Probes = []cschema.Probe{{Id: "ping"}}

	flow := conn.s.buildManifestSetupFlow(conn)
	first, err := flow.FirstStep(context.Background())
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, "apxy:verify", first.Id())
}

func TestManifestSetupFlowSkipsVerifyWhenAllProbesDisabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn, _ := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{
		Configure: &cschema.SetupFlowPhase{
			Steps: []cschema.SetupFlowStep{
				{Id: "workspace", JsonSchema: workspaceSchema},
			},
		},
	})
	setConnectionConfigFixture(t, conn, map[string]any{"run_probe": false})
	conn.connector.GetDefinition().Probes = []cschema.Probe{{
		Id: "ping",
		If: &common.Predicate{Javascript: `cfg.run_probe === true`},
	}}

	flow := conn.s.buildManifestSetupFlow(conn)
	first, err := flow.FirstStep(context.Background())
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, "workspace", first.Id())
	assert.True(t, flow.ContainsStep("apxy:verify"))

	verify, ok, err := flow.StepById(context.Background(), "apxy:verify")
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Nil(t, verify)
}

func TestManifestSetupFlowIfJavascriptError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn, _ := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{
		Configure: &cschema.SetupFlowPhase{
			Steps: []cschema.SetupFlowStep{
				{
					Id:         "broken",
					JsonSchema: workspaceSchema,
					If:         &common.Predicate{Javascript: `cfg.region ===`},
				},
			},
		},
	})

	flow := conn.s.buildManifestSetupFlow(conn)
	_, err := flow.FirstStep(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "if.javascript")
}

// TestSetupStepPredicateIsolation checks both step kinds and traversal with
// fresh metadata snapshots, even when an earlier predicate mutates its inputs.
func TestSetupStepPredicateIsolation(t *testing.T) {
	conn := newTestConnection(cschema.ConnectorDefinition{
		Javascript: `function configuredRegion() { return cfg.region; }`,
	})
	conn.configLoaded, conn.configCache = true, map[string]any{"region": "eu"}
	conn.Labels = map[string]string{"team": "platform"}
	conn.Annotations = map[string]string{"note": "original"}
	specs := []cschema.SetupFlowStep{
		{Id: "hidden-form", If: &common.Predicate{Javascript: "false"}},
		{Id: "hidden-redirect", Type: cschema.SetupFlowStepTypeRedirect, If: &common.Predicate{Javascript: "false"}},
		{Id: "mutate", Type: cschema.SetupFlowStepTypeRedirect, If: &common.Predicate{Javascript: `
			cfg.region = "changed"; labels.team = "changed"; annotations.note = "changed"; true;
		`}},
		{Id: "original", If: &common.Predicate{Javascript: `
			configuredRegion() === "eu" && labels.team === "platform" && annotations.note === "original"
		`}},
	}
	flow := &manifestFlow{steps: make([]iface.ManifestSetupStep, len(specs))}
	for i := range specs {
		flow.steps[i] = conn.s.newSchemaStep(conn, &specs[i])
	}

	// Both step kinds are filtered, and the later form sees the original data.
	steps, err := flow.Steps(context.Background())
	require.NoError(t, err)
	require.Len(t, steps, 2)
	require.Equal(t, "mutate", steps[0].Id())
	require.Equal(t, "original", steps[1].Id())
	first, err := flow.FirstStep(context.Background())
	require.NoError(t, err)
	require.Equal(t, "mutate", first.Id())
	next, ok, err := flow.NextStep(context.Background(), first.Id())
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "original", next.Id())

	// Traversal never writes authored mutations back to connection state.
	require.Equal(t, "eu", conn.configCache["region"])
	require.Equal(t, "platform", conn.Labels["team"])
	require.Equal(t, "original", conn.Annotations["note"])
	require.Nil(t, newSchemaStepEligibility(conn, &cschema.SetupFlowStep{Id: "unconditional"}))
}

// TestSetupStepPredicateFailures exercises the bounded evaluator through setup
// eligibility while preserving step identity and withholding authored details.
func TestSetupStepPredicateFailures(t *testing.T) {
	for name, test := range map[string]struct {
		library   string
		condition string
		config    map[string]any
		want      error
	}{
		"library syntax":                {library: "const privateCompileDetail = ;", condition: "true", want: jsruntime.ErrInvalidSource},
		"condition syntax":              {condition: "privateCompileDetail ===", want: jsruntime.ErrInvalidSource},
		"library throw":                 {library: `throw new Error("privateLibraryDetail")`, condition: "true", want: jsruntime.ErrEvaluation},
		"condition throw":               {condition: `throw {message: "privateConditionDetail"};`, want: jsruntime.ErrEvaluation},
		"helper initialization timeout": {library: "for (;;) {}", condition: "true", want: jsruntime.ErrLimitExceeded},
		"input bytes": {
			condition: "true", config: map[string]any{"padding": strings.Repeat("x", apjs.ConnectorPredicateLimits().MaxInputBytes)},
			want: jsruntime.ErrLimitExceeded,
		},
		"combined source bytes": {
			library: strings.Repeat(" ", apjs.ConnectorPredicateLimits().MaxSourceBytes), condition: "true",
			want: jsruntime.ErrLimitExceeded,
		},
	} {
		t.Run(name, func(t *testing.T) {
			conn := newTestConnection(cschema.ConnectorDefinition{Javascript: test.library})
			conn.configLoaded, conn.configCache = true, test.config
			eligible := newSchemaStepEligibility(conn, &cschema.SetupFlowStep{
				Id: "bounded", If: &common.Predicate{Javascript: test.condition},
			})

			// Policy limits and safe error categories survive the setup-step wrapper.
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			ok, err := eligible(ctx)
			require.ErrorIs(t, err, test.want)
			require.False(t, ok)
			require.Contains(t, err.Error(), `step "bounded" if.javascript`)
			require.NotContains(t, err.Error(), "private")
			require.NoError(t, ctx.Err())
			if name == "combined source bytes" {
				// The size gate runs before even an empty helper program is compiled.
				require.False(t, conn.connector.jsLoaded)
			}
		})
	}
}

// TestSetupStepPredicateCancellation preserves the caller's cancellation before
// compilation and its deadline while a setup predicate is executing.
func TestSetupStepPredicateCancellation(t *testing.T) {
	t.Run("before compilation", func(t *testing.T) {
		conn := newTestConnection(cschema.ConnectorDefinition{Javascript: "const privateCompileDetail = ;"})
		eligible := newSchemaStepEligibility(conn, &cschema.SetupFlowStep{
			Id: "canceled", If: &common.Predicate{Javascript: "true"},
		})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// Canceled requests neither compile source nor turn cancellation into a syntax error.
		ok, err := eligible(ctx)
		require.ErrorIs(t, err, context.Canceled)
		require.False(t, ok)
		require.False(t, conn.connector.jsLoaded)
	})
	t.Run("during evaluation", func(t *testing.T) {
		conn := newTestConnection(cschema.ConnectorDefinition{})
		conn.configLoaded = true
		eligible := newSchemaStepEligibility(conn, &cschema.SetupFlowStep{
			Id: "canceled", If: &common.Predicate{Javascript: "for (;;) {}"},
		})
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		// The caller's deadline ends the loop before the evaluator's one-second timeout.
		ok, err := eligible(ctx)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.False(t, ok)
	})
}

// TestSetupStepPredicateDefinitionFailure preserves infrastructure error causes
// and the caller's context when the connector definition has not been cached.
func TestSetupStepPredicateDefinitionFailure(t *testing.T) {
	conn := newTestConnection(cschema.ConnectorDefinition{})
	conn.configLoaded = true
	encryption := encryptmock.NewMockE(gomock.NewController(t))
	conn.s.encrypt = encryption
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	loadFailure := errors.New("transient definition decryption failure")
	encryption.EXPECT().DecryptString(ctx, conn.connector.EncryptedDefinition).Return("", loadFailure)
	eligible := newSchemaStepEligibility(conn, &cschema.SetupFlowStep{
		Id: "conditional", If: &common.Predicate{Javascript: "true"},
	})

	// Failed loading remains retryable and is not reported as invalid JavaScript.
	ok, err := eligible(ctx)
	require.ErrorIs(t, err, loadFailure)
	require.NotErrorIs(t, err, jsruntime.ErrInvalidSource)
	require.False(t, ok)
	require.Contains(t, err.Error(), `step "conditional" if.javascript`)
	require.Nil(t, conn.connector.def)
	require.False(t, conn.connector.jsLoaded)
}

// setConnectionConfigFixture stores encrypted configuration for setup-flow tests.
func setConnectionConfigFixture(t *testing.T, conn *connection, cfg map[string]any) {
	t.Helper()

	data, err := json.Marshal(cfg)
	require.NoError(t, err)

	ef, err := conn.s.encrypt.EncryptStringForNamespace(context.Background(), conn.Namespace, string(data))
	require.NoError(t, err)
	conn.EncryptedConfiguration = &ef
}
