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
	generation := uint64(1)

	dbMock.
		EXPECT().
		GetConnection(gomock.Any(), connUuuid).
		Return(&database.Connection{
			Id:                  connUuuid,
			State:               database.ConnectionStateConfigured,
			ConnectorId:         connectorID,
			ConnectorGeneration: generation,
			CreatedAt:           clock.Now(),
			UpdatedAt:           clock.Now(),
		}, nil).
		AnyTimes()

	mockConnectorDefinitionRetrieval(ctx, dbMock, e, connectorID, generation, database.ConnectorGenerationStatePrimary, nil, nil, definition)
}

// MockConnectorRetrival sets up mocks to retrieve a connector from the service any number of times.
func MockConnectorRetrival(ctx context.Context, dbMock *mockDb.MockDB, e *mockE.MockE, c *cschema.Connector) {
	state := database.ConnectorGenerationStatePrimary
	if c.Spec.Release.DesiredState != "" {
		state = database.ConnectorGenerationState(c.Spec.Release.DesiredState)
	}
	connectorID := c.GetId()

	mockConnectorDefinitionRetrieval(ctx, dbMock, e, connectorID, c.Metadata.Generation, state, c.Metadata.Labels, c.Metadata.Annotations, &c.Spec.Definition)
}

func mockConnectorDefinitionRetrieval(ctx context.Context, dbMock *mockDb.MockDB, e *mockE.MockE, connectorID apid.ID, generation uint64, state database.ConnectorGenerationState, labels, annotations map[string]string, definition *cschema.ConnectorDefinition) {
	clock := apctx.GetClock(ctx)
	encryptedDefinition := encfield.EncryptedField{
		ID:   "dek_mock",
		Data: fmt.Sprintf("%s-encrypted-definition", connectorID.String()),
	}

	dbMock.
		EXPECT().
		GetConnectorGeneration(gomock.Any(), connectorID, generation).
		Return(&database.ConnectorWithDefinition{
			Id:                  connectorID,
			Generation:          generation,
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
