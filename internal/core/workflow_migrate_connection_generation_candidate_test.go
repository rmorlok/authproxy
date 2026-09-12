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
	connectorID := apid.New(apid.PrefixConnector)
	sourceDB := migrationTestDBConnectorGeneration(
		t,
		e,
		connectorID,
		1,
		database.ConnectorGenerationStateActive,
		cschema.ConnectorDefinition{
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
	targetDB := migrationTestDBConnectorGeneration(
		t,
		e,
		connectorID,
		2,
		database.ConnectorGenerationStateActive,
		cschema.ConnectorDefinition{
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
		Id:                  connID,
		Namespace:           "root",
		State:               database.ConnectionStateConfigured,
		HealthState:         database.ConnectionHealthStateHealthy,
		ConnectorId:         connectorID,
		ConnectorGeneration: 1,
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
	db.EXPECT().GetConnectorGeneration(gomock.Any(), connectorID, uint64(1)).Return(sourceDB, nil).AnyTimes()
	db.EXPECT().GetConnectorGeneration(gomock.Any(), connectorID, uint64(2)).Return(targetDB, nil).AnyTimes()

	candidate, err := s.buildConnectionMigrationCandidate(context.Background(), connID, 2)
	require.NoError(t, err)
	require.Equal(t, targetDB.Generation, candidate.Target.Generation)
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
	connectorID := apid.New(apid.PrefixConnector)

	t.Run("same generation", func(t *testing.T) {
		db.EXPECT().GetConnection(gomock.Any(), connID).Return(&database.Connection{
			Id:                  connID,
			Namespace:           "root",
			State:               database.ConnectionStateConfigured,
			ConnectorId:         connectorID,
			ConnectorGeneration: 1,
		}, nil)
		db.EXPECT().GetConnectorGeneration(gomock.Any(), connectorID, uint64(1)).Return(
			migrationTestDBConnectorGeneration(t, e, connectorID, 1, database.ConnectorGenerationStateActive, cschema.ConnectorDefinition{}),
			nil,
		)

		_, err := s.buildConnectionMigrationCandidate(context.Background(), connID, 1)
		require.ErrorContains(t, err, "already on connector generation")
	})

	t.Run("archived target", func(t *testing.T) {
		db.EXPECT().GetConnection(gomock.Any(), connID).Return(&database.Connection{
			Id:                  connID,
			Namespace:           "root",
			State:               database.ConnectionStateConfigured,
			ConnectorId:         connectorID,
			ConnectorGeneration: 1,
		}, nil)
		db.EXPECT().GetConnectorGeneration(gomock.Any(), connectorID, uint64(1)).Return(
			migrationTestDBConnectorGeneration(t, e, connectorID, 1, database.ConnectorGenerationStateActive, cschema.ConnectorDefinition{}),
			nil,
		)
		db.EXPECT().GetConnectorGeneration(gomock.Any(), connectorID, uint64(2)).Return(
			migrationTestDBConnectorGeneration(t, e, connectorID, 2, database.ConnectorGenerationStateArchived, cschema.ConnectorDefinition{}),
			nil,
		)

		_, err := s.buildConnectionMigrationCandidate(context.Background(), connID, 2)
		require.ErrorContains(t, err, "target connector generation must be primary or active")
	})
}

func TestMigrationGenerationPathOrdersUpAndDown(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := mockDb.NewMockDB(ctrl)
	s, e := newMigrationTestService(t, db)
	connectorID := apid.New(apid.PrefixConnector)
	for generation := uint64(1); generation <= 3; generation++ {
		db.EXPECT().
			GetConnectorGeneration(gomock.Any(), connectorID, generation).
			Return(migrationTestDBConnectorGeneration(t, e, connectorID, generation, database.ConnectorGenerationStateActive, cschema.ConnectorDefinition{}), nil).
			AnyTimes()
	}

	up, err := s.migrationGenerationPath(context.Background(), connectorID, 1, 3)
	require.NoError(t, err)
	require.Equal(t, []uint64{2, 3}, migrationTestGenerationNumbers(up))

	down, err := s.migrationGenerationPath(context.Background(), connectorID, 3, 1)
	require.NoError(t, err)
	require.Equal(t, []uint64{3, 2}, migrationTestGenerationNumbers(down))
}

func TestMigrationGenerationPathPropagatesLookupError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := mockDb.NewMockDB(ctrl)
	s, _ := newMigrationTestService(t, db)
	connectorID := apid.New(apid.PrefixConnector)
	wantErr := errors.New("missing generation")
	db.EXPECT().GetConnectorGeneration(gomock.Any(), connectorID, uint64(2)).Return(nil, wantErr)

	_, err := s.migrationGenerationPath(context.Background(), connectorID, 1, 2)
	require.ErrorIs(t, err, wantErr)
}

func migrationTestGenerationNumbers(generations []*Connector) []uint64 {
	result := make([]uint64, 0, len(generations))
	for _, generation := range generations {
		result = append(result, generation.Generation)
	}
	return result
}
