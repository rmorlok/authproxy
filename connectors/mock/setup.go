package mock

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/apctx"
	cfg "github.com/rmorlok/authproxy/config/connectors"
	"github.com/rmorlok/authproxy/database"
	mockDb "github.com/rmorlok/authproxy/database/mock"
	mockE "github.com/rmorlok/authproxy/encrypt/mock"
)

// MockConnectionRetrieval sets up the service to retrieve a connection with an associated connector any number of times
func MockConnectionRetrieval(ctx context.Context, dbMock *mockDb.MockDB, e *mockE.MockE, connUuuid uuid.UUID, c *cfg.Connector) {
	clock := apctx.GetClock(ctx)

	dbMock.
		EXPECT().
		GetConnection(gomock.Any(), connUuuid).
		Return(&database.Connection{
			ID:               connUuuid,
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
func MockConnectorRetrival(ctx context.Context, dbMock *mockDb.MockDB, e *mockE.MockE, c *cfg.Connector) {
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
			ID:                  c.Id,
			Version:             c.Version,
			State:               state,
			Type:                c.Type,
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
			mockDb.ConnectorVersionMatcher{
				ExpectedId:      c.Id,
				ExpectedVersion: c.Version,
			},
			encryptedDefinition).
		Return(string(connJson), nil).
		AnyTimes()
}
