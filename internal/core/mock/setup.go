package mock

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	mockDb "github.com/rmorlok/authproxy/internal/database/mock"
	"github.com/rmorlok/authproxy/internal/encfield"
	mockE "github.com/rmorlok/authproxy/internal/encrypt/mock"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
)

// MockConnectionRetrieval sets up the service to retrieve a connection with an associated connector any number of times
func MockConnectionRetrieval(ctx context.Context, dbMock *mockDb.MockDB, e *mockE.MockE, connUuuid apid.ID, definition *cschema.ConnectorDefinition) {
	clock := apctx.GetClock(ctx)
	connectorID := apid.New(apid.PrefixConnector)
	version := uint64(1)

	dbMock.
		EXPECT().
		GetConnection(gomock.Any(), connUuuid).
		Return(&database.Connection{
			Id:               connUuuid,
			State:            database.ConnectionStateConfigured,
			ConnectorId:      connectorID,
			ConnectorVersion: version,
			CreatedAt:        clock.Now(),
			UpdatedAt:        clock.Now(),
		}, nil).
		AnyTimes()

	mockConnectorDefinitionRetrieval(ctx, dbMock, e, connectorID, version, database.ConnectorDefinitionVersionStatePrimary, nil, nil, definition)
}

// MockConnectorRetrival sets up mocks to retrieve a connector from the service any number of times.
func MockConnectorRetrival(ctx context.Context, dbMock *mockDb.MockDB, e *mockE.MockE, c *cschema.Connector) {
	state := database.ConnectorDefinitionVersionStatePrimary
	if c.Spec.Release.DesiredState != "" {
		state = database.ConnectorDefinitionVersionState(c.Spec.Release.DesiredState)
	}
	connectorID := c.GetId()

	mockConnectorDefinitionRetrieval(ctx, dbMock, e, connectorID, c.Metadata.Generation, state, c.Metadata.Labels, c.Metadata.Annotations, &c.Spec.Definition)
}

func mockConnectorDefinitionRetrieval(ctx context.Context, dbMock *mockDb.MockDB, e *mockE.MockE, connectorID apid.ID, version uint64, state database.ConnectorDefinitionVersionState, labels, annotations map[string]string, definition *cschema.ConnectorDefinition) {
	clock := apctx.GetClock(ctx)
	encryptedDefinition := encfield.EncryptedField{
		ID:   "dek_mock",
		Data: fmt.Sprintf("%s-encrypted-definition", connectorID.String()),
	}

	dbMock.
		EXPECT().
		GetConnectorDefinitionVersion(gomock.Any(), connectorID, version).
		Return(&database.ConnectorWithDefinition{
			Id:                  connectorID,
			Version:             version,
			State:               state,
			Labels:              labels,
			Annotations:         annotations,
			EncryptedDefinition: encryptedDefinition,
			CreatedAt:           clock.Now(),
			UpdatedAt:           clock.Now(),
		}, nil).
		AnyTimes()

	connJson, err := json.Marshal(definition)
	if err != nil {
		panic(err)
	}

	e.
		EXPECT().
		DecryptString(
			gomock.Any(),
			encryptedDefinition).
		Return(string(connJson), nil).
		AnyTimes()
}
