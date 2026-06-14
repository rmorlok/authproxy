package core

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/golang/mock/gomock"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManifestSetupFlowIfJavascript(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn, _ := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{
		Configure: &cschema.SetupFlowPhase{
			Steps: []cschema.SetupFlowStep{
				{
					Id:         "cfg_false",
					JsonSchema: workspaceSchema,
					If:         &cschema.SetupFlowStepIf{Javascript: `cfg.region === "us"`},
				},
				{
					Id:         "label_true",
					JsonSchema: workspaceSchema,
					If:         &cschema.SetupFlowStepIf{Javascript: `labels["apxy/cxr/type"] === "salesforce"`},
				},
				{
					Id:         "annotation_true",
					JsonSchema: workspaceSchema,
					If:         &cschema.SetupFlowStepIf{Javascript: `annotations["setup-mode"] === "advanced"`},
				},
			},
		},
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
					If:         &cschema.SetupFlowStepIf{Javascript: `cfg.region === "us"`},
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
	conn.cv.GetDefinition().Probes = []cschema.Probe{{Id: "ping"}}

	flow := conn.s.buildManifestSetupFlow(conn)
	first, err := flow.FirstStep(context.Background())
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, "apxy:verify", first.Id())
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
					If:         &cschema.SetupFlowStepIf{Javascript: `cfg.region ===`},
				},
			},
		},
	})

	flow := conn.s.buildManifestSetupFlow(conn)
	_, err := flow.FirstStep(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "if.javascript")
}

func setConnectionConfigFixture(t *testing.T, conn *connection, cfg map[string]any) {
	t.Helper()

	data, err := json.Marshal(cfg)
	require.NoError(t, err)

	ef, err := conn.s.encrypt.EncryptStringForNamespace(context.Background(), conn.Namespace, string(data))
	require.NoError(t, err)
	conn.EncryptedConfiguration = &ef
}
