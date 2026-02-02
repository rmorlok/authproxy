package mock

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/database"
	mockDb "github.com/rmorlok/authproxy/internal/database/mock"
	mockE "github.com/rmorlok/authproxy/internal/encrypt/mock"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
)

// MockConnectionRetrieval sets up the service to retrieve a connection with an associated connector any number of times
func MockConnectionRetrieval(ctx context.Context, dbMock *mockDb.MockDB, e *mockE.MockE, connUuuid uuid.UUID, c *cschema.Connector) {
	clock := apctx.GetClock(ctx)

	dbMock.
		EXPECT().
		GetConnection(gomock.Any(), connUuuid).
		Return(&database.Connection{
			Id:               connUuuid,
			State:            database.ConnectionStateReady,
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
	state := database.ConnectorVersionStatePrimary
	if c.State != "" {
		state = database.ConnectorVersionState(c.State)
	}

	clock := apctx.GetClock(ctx)
	hash := fmt.Sprintf("%s-hash", c.Id.String())
	encryptedDefinition := fmt.Sprintf("%s-encrypted-definition", c.Id.String())

	dbMock.
		EXPECT().
		GetConnectorVersion(gomock.Any(), c.Id, c.Version).
		Return(&database.ConnectorVersion{
			Id:                  c.Id,
			Version:             c.Version,
			State:               state,
			Labels:              c.Labels,
			Hash:                hash,
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
		DecryptStringForConnector(
			gomock.Any(),
			ConnectorVersionMatcher{
				ExpectedId:      c.Id,
				ExpectedVersion: c.Version,
			},
			encryptedDefinition).
		Return(string(connJson), nil).
		AnyTimes()
}
