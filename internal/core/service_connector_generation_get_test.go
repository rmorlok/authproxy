package core

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/mock"
	"github.com/rmorlok/authproxy/internal/database"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/require"
)

func TestGetConnectorGeneration(t *testing.T) {
	ctrl := gomock.NewController(t)
	s, db, _, _, _, e := FullMockService(t, ctrl)

	id := apid.New(apid.PrefixActor)
	generation := uint64(1)

	mock.MockConnectorRetrival(context.Background(), db, e, connectorResourceForMock(id, generation, database.ConnectorGenerationStatePrimary, map[string]string{"type": "test"}, cschema.ConnectorDefinition{
		DisplayName: "Test Connector",
		Auth:        cschema.NewNoAuth(),
	}))

	c, err := s.GetConnectorGeneration(context.Background(), id, generation)
	require.NoError(t, err)
	require.Equal(t, c.GetLabels()["type"], "test")
}
