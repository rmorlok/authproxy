package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/auth_methods/oauth2"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/common"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestOAuth2ConnectionAtAuthStep builds a connection on the OAuth2
// redirect step — i.e. the state the OAuth2 callback handler observes when
// it finishes token exchange and calls HandleCredentialsEstablished. The
// supplied setup flow's preconnect/configure / probes drive what the next
// step resolves to.
func newTestOAuth2ConnectionAtAuthStep(t *testing.T, ctrl *gomock.Controller, sf *cschema.SetupFlow, probes []cschema.Probe) (*connection, *struct {
	db *struct{}
}, *struct{}) {
	t.Helper()
	conn, db, ac := newTestConnectionWithSetupFlowAndAsynq(t, ctrl, sf)
	// Replace the no-auth connector built by the helper with an OAuth2 one
	// so the manifest emits the apxy:auth:oauth2_authorize step that
	// HandleCredentialsEstablished can transition out of.
	connector := cschema.Connector{
		Auth:      &cschema.Auth{InnerVal: &cschema.AuthOAuth2{Type: cschema.AuthTypeOAuth2}},
		SetupFlow: sf,
		Probes:    probes,
	}
	cv := NewTestConnectorVersion(connector)
	conn.cv = cv
	conn.ConnectorId = cv.GetId()
	conn.ConnectorVersion = cv.GetVersion()
	// Position the connection at the OAuth2 redirect step, as it would be
	// after SetStateAndGeneratePublicUrl ran and before the callback fires.
	step := cschema.MustNewSetupStep(oauth2.OAuth2AuthorizeStepId)
	conn.SetupStep = &step
	// Returning typed _ wrappers for db/ac would obscure usage; reuse the
	// originals — Go's interfaces below let callers ignore the wrapper.
	_ = db
	_ = ac
	return conn, nil, nil
}

func TestHandleCredentialsEstablished(t *testing.T) {
	t.Run("enters verify and enqueues task when connector has probes", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, db, ac := newTestConnectionWithSetupFlowAndAsynq(t, ctrl, &cschema.SetupFlow{})
		// Promote the no-auth connector to OAuth2 + probes so the manifest
		// emits the auth step the callback observes, and has a verify slot.
		connector := cschema.Connector{
			Auth:   &cschema.Auth{InnerVal: &cschema.AuthOAuth2{Type: cschema.AuthTypeOAuth2}},
			Probes: []cschema.Probe{{Id: "ping"}},
		}
		conn.cv = NewTestConnectorVersion(connector)
		conn.ConnectorId = conn.cv.GetId()
		conn.ConnectorVersion = conn.cv.GetVersion()
		current := cschema.MustNewSetupStep(oauth2.OAuth2AuthorizeStepId)
		conn.SetupStep = &current

		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, ptrStep(cschema.SetupStepVerify)).Return(nil)
		ac.EXPECT().
			EnqueueContext(gomock.Any(), gomock.AssignableToTypeOf(&asynq.Task{}), asynq.Retention(10*time.Minute)).
			Return(&asynq.TaskInfo{ID: "mock-task-id"}, nil)

		outcome, err := conn.HandleCredentialsEstablished(context.Background())
		require.NoError(t, err)
		assert.True(t, outcome.SetupPending)
		require.NotNil(t, conn.GetSetupStep())
		assert.Equal(t, cschema.SetupStepVerify, *conn.GetSetupStep())
	})

	t.Run("enters first configure when connector has configure but no probes", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		sf := &cschema.SetupFlow{
			Configure: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{
					{Id: "workspace", JsonSchema: workspaceSchema},
				},
			},
		}
		conn, db := newTestConnectionWithSetupFlow(t, ctrl, sf)
		// OAuth2 connector positions us at the auth step.
		conn.cv = NewTestConnectorVersion(cschema.Connector{
			Auth:      &cschema.Auth{InnerVal: &cschema.AuthOAuth2{Type: cschema.AuthTypeOAuth2}},
			SetupFlow: sf,
		})
		conn.ConnectorId = conn.cv.GetId()
		conn.ConnectorVersion = conn.cv.GetVersion()
		current := cschema.MustNewSetupStep(oauth2.OAuth2AuthorizeStepId)
		conn.SetupStep = &current

		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, ptrStep(cschema.MustNewSetupStep("workspace"))).Return(nil)

		outcome, err := conn.HandleCredentialsEstablished(context.Background())
		require.NoError(t, err)
		assert.True(t, outcome.SetupPending)
		require.NotNil(t, conn.GetSetupStep())
		assert.Equal(t, "workspace", conn.GetSetupStep().String())
	})

	t.Run("skips verify when all probes are disabled", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		sf := &cschema.SetupFlow{
			Configure: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{
					{Id: "workspace", JsonSchema: workspaceSchema},
				},
			},
		}
		conn, db, _ := newTestConnectionWithSetupFlowAndAsynq(t, ctrl, sf)
		conn.cv = NewTestConnectorVersion(cschema.Connector{
			Auth:      &cschema.Auth{InnerVal: &cschema.AuthOAuth2{Type: cschema.AuthTypeOAuth2}},
			SetupFlow: sf,
			Probes: []cschema.Probe{{
				Id: "ping",
				If: &common.Predicate{Javascript: `cfg.run_probe === true`},
			}},
		})
		conn.ConnectorId = conn.cv.GetId()
		conn.ConnectorVersion = conn.cv.GetVersion()
		setConnectionConfigFixture(t, conn, map[string]any{"run_probe": false})
		current := cschema.MustNewSetupStep(oauth2.OAuth2AuthorizeStepId)
		conn.SetupStep = &current

		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, ptrStep(cschema.MustNewSetupStep("workspace"))).Return(nil)
		// No EnqueueContext expectation — disabled probes must skip verify.

		outcome, err := conn.HandleCredentialsEstablished(context.Background())
		require.NoError(t, err)
		assert.True(t, outcome.SetupPending)
		require.NotNil(t, conn.GetSetupStep())
		assert.Equal(t, "workspace", conn.GetSetupStep().String())
	})

	t.Run("clears setup step and marks ready when connector has neither probes nor configure", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, db := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{})
		conn.cv = NewTestConnectorVersion(cschema.Connector{
			Auth: &cschema.Auth{InnerVal: &cschema.AuthOAuth2{Type: cschema.AuthTypeOAuth2}},
		})
		conn.ConnectorId = conn.cv.GetId()
		conn.ConnectorVersion = conn.cv.GetVersion()
		current := cschema.MustNewSetupStep(oauth2.OAuth2AuthorizeStepId)
		conn.SetupStep = &current

		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, (*cschema.SetupStep)(nil)).Return(nil)
		db.EXPECT().SetConnectionState(gomock.Any(), conn.Id, database.ConnectionStateConfigured).Return(nil)

		outcome, err := conn.HandleCredentialsEstablished(context.Background())
		require.NoError(t, err)
		assert.False(t, outcome.SetupPending)
		assert.Nil(t, conn.GetSetupStep())
		assert.Equal(t, database.ConnectionStateConfigured, conn.GetState())
	})

	t.Run("rejects call when connection has no active setup step", func(t *testing.T) {
		// The OAuth2 callback path always sets setup_step before invoking
		// HandleCredentialsEstablished. A nil step is a programmer error.
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, _ := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{})
		_, err := conn.HandleCredentialsEstablished(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no active setup step")
	})
}

func TestHandleAuthFailed(t *testing.T) {
	t.Run("records error and moves to auth_failed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, db := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{})

		db.EXPECT().SetConnectionSetupError(gomock.Any(), conn.Id, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ any, msg *string) error {
				require.NotNil(t, msg)
				assert.Contains(t, *msg, "token exchange boom")
				return nil
			})
		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, ptrStep(cschema.SetupStepAuthFailed)).Return(nil)

		err := conn.HandleAuthFailed(context.Background(), errors.New("token exchange boom"))
		require.NoError(t, err)
		require.NotNil(t, conn.GetSetupStep())
		assert.Equal(t, cschema.SetupStepAuthFailed, *conn.GetSetupStep())
		require.NotNil(t, conn.GetSetupError())
		assert.Contains(t, *conn.GetSetupError(), "token exchange boom")
	})
}
