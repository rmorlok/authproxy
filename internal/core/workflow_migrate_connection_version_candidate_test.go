package core

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	mockDb "github.com/rmorlok/authproxy/internal/database/mock"
	"github.com/rmorlok/authproxy/internal/schema/common"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/require"
)

func TestBuildConnectionMigrationCandidateAssemblesTargetState(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := mockDb.NewMockDB(ctrl)
	s, e := newMigrationTestService(t, db)
	connID := apid.New(apid.PrefixConnection)
	connectorID := apid.New(apid.PrefixConnectorVersion)
	sourceDB := migrationTestDBConnectorVersion(
		t,
		e,
		connectorID,
		1,
		database.ConnectorVersionStateActive,
		cschema.Connector{
			Auth: cschema.NewNoAuth(),
			SetupFlow: &cschema.SetupFlow{
				Configure: &cschema.SetupFlowPhase{
					Steps: []cschema.SetupFlowStep{{
						Id: "existing",
						JsonSchema: common.RawJSON(`{
							"type": "object",
							"properties": {"existing": {"type": "string"}}
						}`),
					}},
				},
			},
		},
	)
	targetDB := migrationTestDBConnectorVersion(
		t,
		e,
		connectorID,
		2,
		database.ConnectorVersionStateActive,
		cschema.Connector{
			Auth: cschema.NewNoAuth(),
			Probes: []cschema.Probe{
				{Id: "existing-probe"},
				{Id: "added-probe"},
			},
			SetupFlow: &cschema.SetupFlow{
				Configure: &cschema.SetupFlowPhase{
					Steps: []cschema.SetupFlowStep{{
						Id: "configure",
						JsonSchema: common.RawJSON(`{
							"type": "object",
							"required": ["workspace"],
							"properties": {
								"existing": {"type": "string"},
								"region": {"type": "string", "default": "us"},
								"workspace": {"type": "string"}
							}
						}`),
					}},
				},
			},
		},
	)
	db.EXPECT().GetConnection(gomock.Any(), connID).Return(&database.Connection{
		Id:               connID,
		Namespace:        "root",
		State:            database.ConnectionStateConfigured,
		HealthState:      database.ConnectionHealthStateHealthy,
		ConnectorId:      connectorID,
		ConnectorVersion: 1,
		Labels: database.Labels{
			"team":            "platform",
			"apxy/cxn/-/type": "owned",
		},
		Annotations: map[string]string{
			"note": "keep",
		},
		EncryptedConfiguration: migrationTestEncryptedConfig(t, e, "root", map[string]any{
			"existing": "value",
		}),
	}, nil)
	db.EXPECT().GetConnectorVersion(gomock.Any(), connectorID, uint64(1)).Return(sourceDB, nil).AnyTimes()
	db.EXPECT().GetConnectorVersion(gomock.Any(), connectorID, uint64(2)).Return(targetDB, nil).AnyTimes()

	candidate, err := s.buildConnectionMigrationCandidate(context.Background(), connID, 2)
	require.NoError(t, err)
	require.Equal(t, targetDB.Version, candidate.Target.Version)
	require.Equal(t, map[string]any{"existing": "value", "region": "us"}, candidate.Config)
	require.Equal(t, map[string]string{"team": "platform"}, candidate.UserLabels)
	require.Equal(t, map[string]string{"note": "keep"}, candidate.Annotations)
	require.Equal(t, []string{"existing-probe", "added-probe"}, candidate.ProbeIdsToRun)
	require.NotNil(t, candidate.SetupStep)
	require.Equal(t, "configure", candidate.SetupStep.Id())
	require.Len(t, candidate.Notifications, 1)
	require.Equal(t, connectionNotificationKey(candidate, database.NotificationKeySetupRequired), candidate.Notifications[0].Key)
}

func TestBuildConnectionMigrationCandidateRejectsNoopAndInactiveTargets(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := mockDb.NewMockDB(ctrl)
	s, e := newMigrationTestService(t, db)
	connID := apid.New(apid.PrefixConnection)
	connectorID := apid.New(apid.PrefixConnectorVersion)

	t.Run("same version", func(t *testing.T) {
		db.EXPECT().GetConnection(gomock.Any(), connID).Return(&database.Connection{
			Id:               connID,
			Namespace:        "root",
			State:            database.ConnectionStateConfigured,
			ConnectorId:      connectorID,
			ConnectorVersion: 1,
		}, nil)
		db.EXPECT().GetConnectorVersion(gomock.Any(), connectorID, uint64(1)).Return(
			migrationTestDBConnectorVersion(t, e, connectorID, 1, database.ConnectorVersionStateActive, cschema.Connector{}),
			nil,
		)

		_, err := s.buildConnectionMigrationCandidate(context.Background(), connID, 1)
		require.ErrorContains(t, err, "already on connector version")
	})

	t.Run("archived target", func(t *testing.T) {
		db.EXPECT().GetConnection(gomock.Any(), connID).Return(&database.Connection{
			Id:               connID,
			Namespace:        "root",
			State:            database.ConnectionStateConfigured,
			ConnectorId:      connectorID,
			ConnectorVersion: 1,
		}, nil)
		db.EXPECT().GetConnectorVersion(gomock.Any(), connectorID, uint64(1)).Return(
			migrationTestDBConnectorVersion(t, e, connectorID, 1, database.ConnectorVersionStateActive, cschema.Connector{}),
			nil,
		)
		db.EXPECT().GetConnectorVersion(gomock.Any(), connectorID, uint64(2)).Return(
			migrationTestDBConnectorVersion(t, e, connectorID, 2, database.ConnectorVersionStateArchived, cschema.Connector{}),
			nil,
		)

		_, err := s.buildConnectionMigrationCandidate(context.Background(), connID, 2)
		require.ErrorContains(t, err, "target connector version must be primary or active")
	})
}

func TestMigrationVersionPathOrdersUpAndDown(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := mockDb.NewMockDB(ctrl)
	s, e := newMigrationTestService(t, db)
	connectorID := apid.New(apid.PrefixConnectorVersion)
	for version := uint64(1); version <= 3; version++ {
		db.EXPECT().
			GetConnectorVersion(gomock.Any(), connectorID, version).
			Return(migrationTestDBConnectorVersion(t, e, connectorID, version, database.ConnectorVersionStateActive, cschema.Connector{}), nil).
			AnyTimes()
	}

	up, err := s.migrationVersionPath(context.Background(), connectorID, 1, 3)
	require.NoError(t, err)
	require.Equal(t, []uint64{2, 3}, migrationTestVersionNumbers(up))

	down, err := s.migrationVersionPath(context.Background(), connectorID, 3, 1)
	require.NoError(t, err)
	require.Equal(t, []uint64{3, 2}, migrationTestVersionNumbers(down))
}

func TestMigrationVersionPathPropagatesLookupError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := mockDb.NewMockDB(ctrl)
	s, _ := newMigrationTestService(t, db)
	connectorID := apid.New(apid.PrefixConnectorVersion)
	wantErr := errors.New("missing version")
	db.EXPECT().GetConnectorVersion(gomock.Any(), connectorID, uint64(2)).Return(nil, wantErr)

	_, err := s.migrationVersionPath(context.Background(), connectorID, 1, 2)
	require.ErrorIs(t, err, wantErr)
}

func migrationTestVersionNumbers(versions []*ConnectorVersion) []uint64 {
	result := make([]uint64, 0, len(versions))
	for _, version := range versions {
		result = append(result, version.Version)
	}
	return result
}
