package core

import (
	"context"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	cfg "github.com/rmorlok/authproxy/config/connectors"
	"github.com/rmorlok/authproxy/core/mock"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestGetConnectorVersion(t *testing.T) {
	ctrl := gomock.NewController(t)
	s, db, _, _, _, e := FullMockService(t, ctrl)

	id := uuid.New()
	version := uint64(1)

	mock.MockConnectorRetrival(context.Background(), db, e, &cfg.Connector{
		Id:          id,
		Version:     version,
		DisplayName: "Test Connector",
		Type:        "test",
		Auth:        &cfg.AuthApiKey{},
	})

	c, err := s.GetConnectorVersion(context.Background(), id, version)
	require.NoError(t, err)
	require.Equal(t, c.GetType(), "test")
}
