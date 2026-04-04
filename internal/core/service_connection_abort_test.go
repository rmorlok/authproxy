package core

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	mockDb "github.com/rmorlok/authproxy/internal/database/mock"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupAbortTest(t *testing.T, ctrl *gomock.Controller, sf *cschema.SetupFlow, setupStep *string) (*connection, *mockDb.MockDB) {
	conn, db := newTestConnectionWithSetupFlow(t, ctrl, sf)
	conn.SetupStep = setupStep

	// Set up the encrypt service on the main service so getConnection can decrypt the connector version
	conn.s.encrypt = encrypt.NewFakeEncryptService(false)

	// Mock the DB calls that getConnection makes
	db.EXPECT().GetConnection(gomock.Any(), conn.Id).Return(&conn.Connection, nil).AnyTimes()
	db.EXPECT().GetConnectorVersion(gomock.Any(), conn.cv.Id, conn.cv.Version).Return(&database.ConnectorVersion{
		Id:                  conn.cv.Id,
		Version:             conn.cv.Version,
		Labels:              conn.cv.GetLabels(),
		State:               database.ConnectorVersionStatePrimary,
		Hash:                conn.cv.Hash,
		EncryptedDefinition: conn.cv.EncryptedDefinition,
	}, nil).AnyTimes()

	return conn, db
}

func TestAbortConnection(t *testing.T) {
	t.Run("returns error when setup is complete", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, _ := setupAbortTest(t, ctrl, &cschema.SetupFlow{}, nil)

		err := conn.s.AbortConnection(context.Background(), conn.Id)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already complete")
	})

	t.Run("aborts preconnect connection", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		step := "preconnect:0"
		conn, db := setupAbortTest(t, ctrl, &cschema.SetupFlow{
			Preconnect: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{
					{Id: "tenant", JsonSchema: tenantSchema},
				},
			},
		}, &step)

		db.EXPECT().SetConnectionState(gomock.Any(), conn.Id, database.ConnectionStateDisconnected).Return(nil)
		db.EXPECT().DeleteConnection(gomock.Any(), conn.Id).Return(nil)

		err := conn.s.AbortConnection(context.Background(), conn.Id)
		require.NoError(t, err)
	})

	t.Run("aborts auth step connection", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		step := "auth"
		conn, db := setupAbortTest(t, ctrl, &cschema.SetupFlow{
			Preconnect: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{
					{Id: "tenant", JsonSchema: tenantSchema},
				},
			},
		}, &step)

		db.EXPECT().SetConnectionState(gomock.Any(), conn.Id, database.ConnectionStateDisconnected).Return(nil)
		db.EXPECT().DeleteConnection(gomock.Any(), conn.Id).Return(nil)

		err := conn.s.AbortConnection(context.Background(), conn.Id)
		require.NoError(t, err)
	})

	t.Run("aborts configure step connection", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		step := "configure:0"
		conn, db := setupAbortTest(t, ctrl, &cschema.SetupFlow{
			Configure: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{
					{Id: "workspace", JsonSchema: workspaceSchema},
				},
			},
		}, &step)

		db.EXPECT().SetConnectionState(gomock.Any(), conn.Id, database.ConnectionStateDisconnected).Return(nil)
		db.EXPECT().DeleteConnection(gomock.Any(), conn.Id).Return(nil)

		err := conn.s.AbortConnection(context.Background(), conn.Id)
		require.NoError(t, err)
	})
}
