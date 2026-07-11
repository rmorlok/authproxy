package core

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/auth_methods"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	mockDb "github.com/rmorlok/authproxy/internal/database/mock"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/require"
)

type migrationRefreshTestFactory struct {
	auth       auth_methods.Authenticator
	connection iface.Connection
}

func (f *migrationRefreshTestFactory) NewAuthenticator(connection iface.Connection) auth_methods.Authenticator {
	f.connection = connection
	return f.auth
}

func (f *migrationRefreshTestFactory) ManifestSetupSteps(
	iface.Connection,
	*cschema.Connector,
) []iface.ManifestSetupStep {
	return nil
}

type migrationRefreshTestAuthenticator struct {
	refreshCalls int
	recoverCalls int
	refreshErr   error
}

func (a *migrationRefreshTestAuthenticator) Resolve(context.Context) (auth_methods.AuthApplication, error) {
	return auth_methods.AuthApplication{}, nil
}

func (a *migrationRefreshTestAuthenticator) RecoverFrom401(context.Context) error {
	a.recoverCalls++
	return auth_methods.ErrCannotRecover
}

func (a *migrationRefreshTestAuthenticator) Refresh(context.Context) error {
	a.refreshCalls++
	return a.refreshErr
}

func (a *migrationRefreshTestAuthenticator) SupportsRevoke() bool {
	return false
}

func (a *migrationRefreshTestAuthenticator) Revoke(context.Context) error {
	return nil
}

func TestShouldRunMigrationProbesRequiresHealthyMigratedConnection(t *testing.T) {
	step := cschema.MustNewSetupStep("configure")

	tests := []struct {
		name        string
		probeIDs    []string
		setupStep   *cschema.SetupStep
		healthState database.ConnectionHealthState
		want        bool
	}{
		{
			name:        "healthy migrated connection runs probes",
			probeIDs:    []string{"ping"},
			healthState: database.ConnectionHealthStateHealthy,
			want:        true,
		},
		{
			name:        "skips when no probes were selected",
			healthState: database.ConnectionHealthStateHealthy,
		},
		{
			name:        "skips when setup is pending",
			probeIDs:    []string{"ping"},
			setupStep:   &step,
			healthState: database.ConnectionHealthStateHealthy,
		},
		{
			name:        "skips when migration made the connection unhealthy",
			probeIDs:    []string{"ping"},
			healthState: database.ConnectionHealthStateUnhealthy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := &connectionMigrationCandidate{
				ProbeIdsToRun: tt.probeIDs,
				SetupStep:     tt.setupStep,
				HealthState:   tt.healthState,
			}

			require.Equal(t, tt.want, shouldRunMigrationProbes(candidate))
		})
	}
}

func TestRefreshAuthAfterConnectionMigrationCallsRefresh(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := mockDb.NewMockDB(ctrl)
	s, e := newMigrationTestService(t, db)
	connectorID := apid.New(apid.PrefixConnectorVersion)
	targetDB := migrationTestDBConnectorVersion(
		t,
		e,
		connectorID,
		2,
		database.ConnectorVersionStateActive,
		cschema.Connector{Auth: cschema.NewNoAuth()},
	)
	target := wrapConnectorVersion(*targetDB, s)
	auth := &migrationRefreshTestAuthenticator{}
	factory := &migrationRefreshTestFactory{auth: auth}
	s.authMethodFactories = map[cschema.AuthType]auth_methods.Factory{
		cschema.AuthTypeNoAuth: factory,
	}
	updated := &database.Connection{
		Id:               apid.New(apid.PrefixConnection),
		Namespace:        "root",
		State:            database.ConnectionStateConfigured,
		ConnectorId:      connectorID,
		ConnectorVersion: 2,
	}
	db.EXPECT().GetConnectorVersion(gomock.Any(), connectorID, uint64(2)).Return(targetDB, nil)

	err := s.refreshAuthAfterConnectionMigration(context.Background(), updated, &connectionMigrationCandidate{
		Target: target,
	})
	require.NoError(t, err)
	require.Equal(t, 1, auth.refreshCalls)
	require.Zero(t, auth.recoverCalls)
	require.NotNil(t, factory.connection)
	require.Equal(t, updated.Id, factory.connection.GetId())
}

func TestRefreshAuthAfterConnectionMigrationReturnsRefreshError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := mockDb.NewMockDB(ctrl)
	s, e := newMigrationTestService(t, db)
	connectorID := apid.New(apid.PrefixConnectorVersion)
	targetDB := migrationTestDBConnectorVersion(
		t,
		e,
		connectorID,
		2,
		database.ConnectorVersionStateActive,
		cschema.Connector{Auth: cschema.NewNoAuth()},
	)
	target := wrapConnectorVersion(*targetDB, s)
	wantErr := errors.New("refresh failed")
	s.authMethodFactories = map[cschema.AuthType]auth_methods.Factory{
		cschema.AuthTypeNoAuth: &migrationRefreshTestFactory{
			auth: &migrationRefreshTestAuthenticator{refreshErr: wantErr},
		},
	}
	updated := &database.Connection{
		Id:               apid.New(apid.PrefixConnection),
		Namespace:        "root",
		State:            database.ConnectionStateConfigured,
		ConnectorId:      connectorID,
		ConnectorVersion: 2,
	}
	db.EXPECT().GetConnectorVersion(gomock.Any(), connectorID, uint64(2)).Return(targetDB, nil)

	err := s.refreshAuthAfterConnectionMigration(context.Background(), updated, &connectionMigrationCandidate{
		Target: target,
	})
	require.ErrorIs(t, err, wantErr)
}

func TestRefreshAuthAfterConnectionMigrationValidatesFactory(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := mockDb.NewMockDB(ctrl)
	s, e := newMigrationTestService(t, db)
	connectorID := apid.New(apid.PrefixConnectorVersion)
	targetDB := migrationTestDBConnectorVersion(
		t,
		e,
		connectorID,
		2,
		database.ConnectorVersionStateActive,
		cschema.Connector{Auth: cschema.NewNoAuth()},
	)
	target := wrapConnectorVersion(*targetDB, s)
	updated := &database.Connection{
		Id:               apid.New(apid.PrefixConnection),
		Namespace:        "root",
		State:            database.ConnectionStateConfigured,
		ConnectorId:      connectorID,
		ConnectorVersion: 2,
	}

	t.Run("missing factory", func(t *testing.T) {
		db.EXPECT().GetConnectorVersion(gomock.Any(), connectorID, uint64(2)).Return(targetDB, nil)
		err := s.refreshAuthAfterConnectionMigration(context.Background(), updated, &connectionMigrationCandidate{
			Target: target,
		})
		require.ErrorContains(t, err, "auth method factory is not configured")
	})

	t.Run("nil authenticator", func(t *testing.T) {
		s.authMethodFactories = map[cschema.AuthType]auth_methods.Factory{
			cschema.AuthTypeNoAuth: &migrationRefreshTestFactory{},
		}
		db.EXPECT().GetConnectorVersion(gomock.Any(), connectorID, uint64(2)).Return(targetDB, nil)
		err := s.refreshAuthAfterConnectionMigration(context.Background(), updated, &connectionMigrationCandidate{
			Target: target,
		})
		require.ErrorContains(t, err, "authenticator is not configured")
	})
}

func TestEncryptMigrationConfig(t *testing.T) {
	s, e := newMigrationTestService(t, nil)

	encrypted, err := s.encryptMigrationConfig(context.Background(), "root", nil)
	require.NoError(t, err)
	require.Nil(t, encrypted)

	encrypted, err = s.encryptMigrationConfig(context.Background(), "root", map[string]any{
		"tenant": "acme",
		"nested": map[string]any{
			"enabled": true,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, encrypted)
	decrypted, err := e.DecryptString(context.Background(), *encrypted)
	require.NoError(t, err)
	require.JSONEq(t, `{"tenant":"acme","nested":{"enabled":true}}`, decrypted)

	_, err = s.encryptMigrationConfig(context.Background(), "root", map[string]any{
		"bad": make(chan struct{}),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "marshal migrated connection configuration")
}
