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
func MockConnectionRetrieval(ctx context.Context, dbMock *mockDb.MockDB, e *mockE.MockE, connUuuid apid.ID, c *cschema.Connector) {
	clock := apctx.GetClock(ctx)

	dbMock.
		EXPECT().
		GetConnection(gomock.Any(), connUuuid).
		Return(&database.Connection{
			Id:               connUuuid,
			State:            database.ConnectionStateConfigured,
			ConnectorId:      c.Id,
			ConnectorVersion: c.Version,
			CreatedAt:        clock.Now(),
			UpdatedAt:        clock.Now(),
		}, nil).
		AnyTimes()

	MockConnectorRetrival(ctx, dbMock, e, c)
}

// MockConnectorRetrival sets up mocks to retrieve a connector from the service any number of times.
func MockConnectorRetrival(ctx context.Context, dbMock *mockDb.MockDB, e *mockE.MockE, c *cschema.Connector) {
	state := database.ConnectorDefinitionVersionStatePrimary
	if c.State != "" {
		state = database.ConnectorDefinitionVersionState(c.State)
	}

	clock := apctx.GetClock(ctx)
	encryptedDefinition := encfield.EncryptedField{
		ID:   "dek_mock",
		Data: fmt.Sprintf("%s-encrypted-definition", c.Id.String()),
	}

	dbMock.
		EXPECT().
		GetConnectorDefinitionVersion(gomock.Any(), c.Id, c.Version).
		Return(&database.ConnectorWithDefinition{
			Id:                  c.Id,
			Version:             c.Version,
			State:               state,
			Labels:              c.Labels,
			EncryptedDefinition: encryptedDefinition,
			CreatedAt:           clock.Now(),
			UpdatedAt:           clock.Now(),
		}, nil).
		AnyTimes()

	connJson, err := json.Marshal(c)
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
