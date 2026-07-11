package core

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/database"
	mockDb "github.com/rmorlok/authproxy/internal/database/mock"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/encrypt"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/require"
)

func newMigrationTestCandidate(t testing.TB) *connectionMigrationCandidate {
	t.Helper()

	connID := apid.New(apid.PrefixConnection)
	connectorID := apid.New(apid.PrefixConnectorVersion)
	return &connectionMigrationCandidate{
		Connection: &connection{Connection: database.Connection{
			Id:               connID,
			Namespace:        "root",
			ConnectorId:      connectorID,
			ConnectorVersion: 1,
			HealthState:      database.ConnectionHealthStateHealthy,
		}},
		Target: &ConnectorVersion{ConnectorVersion: database.ConnectorVersion{
			Id:      connectorID,
			Version: 2,
		}},
		Config:      map[string]any{},
		UserLabels:  map[string]string{},
		Annotations: map[string]string{},
		HealthState: database.ConnectionHealthStateHealthy,
	}
}

func newMigrationTestService(t testing.TB, db *mockDb.MockDB) (*service, encrypt.E) {
	t.Helper()

	e := encrypt.NewFakeEncryptService(false)
	return &service{
		db:      db,
		encrypt: e,
		logger:  aplog.NewNoopLogger(),
	}, e
}

func migrationTestDBConnectorVersion(
	t testing.TB,
	e encrypt.E,
	connectorID apid.ID,
	version uint64,
	state database.ConnectorVersionState,
	def cschema.Connector,
) *database.ConnectorVersion {
	t.Helper()

	if def.Auth == nil {
		def.Auth = cschema.NewNoAuth()
	}
	def.Id = connectorID
	def.Version = version

	raw, err := json.Marshal(def)
	require.NoError(t, err)
	encrypted, err := e.EncryptStringForEntity(
		context.Background(),
		&namespaceHolder{namespace: "root"},
		string(raw),
	)
	require.NoError(t, err)

	return &database.ConnectorVersion{
		Id:                  connectorID,
		Namespace:           "root",
		Version:             version,
		State:               state,
		Hash:                def.Hash(),
		EncryptedDefinition: encrypted,
	}
}

func migrationTestEncryptedConfig(
	t testing.TB,
	e encrypt.E,
	namespace string,
	cfg map[string]any,
) *encfield.EncryptedField {
	t.Helper()

	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	encrypted, err := e.EncryptStringForNamespace(context.Background(), namespace, string(raw))
	require.NoError(t, err)
	return &encrypted
}

func migrationTestOAuthAuth(scopeIDs ...string) *cschema.Auth {
	scopes := make([]cschema.Scope, 0, len(scopeIDs))
	for _, scopeID := range scopeIDs {
		scopes = append(scopes, cschema.Scope{Id: scopeID})
	}
	return &cschema.Auth{InnerVal: &cschema.AuthOAuth2{
		Type:   cschema.AuthTypeOAuth2,
		Scopes: scopes,
	}}
}
