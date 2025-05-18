package connectors

import (
	"context"
	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/config"
	dbmock "github.com/rmorlok/authproxy/database/mock"
	encryptmock "github.com/rmorlok/authproxy/encrypt/mock"
	"github.com/stretchr/testify/require"
	"log/slog"
	"testing"
)

func TestConnectorsService(t *testing.T) {
	assert := require.New(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := dbmock.NewMockDB(ctrl)
	mockEncrypt := encryptmock.NewMockE(ctrl)
	logger := slog.Default()

	// Create a test configuration with connectors
	cfg := config.FromRoot(&config.Root{
		Connectors: []config.Connector{
			{
				Type:        "test-connector",
				DisplayName: "Test Connector",
				Description: "A test connector",
				Auth: &config.AuthOAuth2{
					Type: config.AuthTypeOAuth2,
				},
			},
		},
	})

	// Create the connectors service
	service := NewConnectorsService(cfg, mockDB, mockEncrypt, logger)
	assert.NotNil(service)

	// Test MigrateConnectors with no connectors
	emptyCfg := config.FromRoot(&config.Root{})
	emptyService := NewConnectorsService(emptyCfg, mockDB, mockEncrypt, logger)
	err := emptyService.MigrateConnectors(context.Background())
	assert.NoError(err)

	// Test MigrateConnectors with connectors
	// Since we're not actually creating connectors in the database yet,
	// we'll just verify that the method doesn't return an error
	// We'll skip the mock expectations for now since we're not actually
	// creating connectors in the database yet

	err = service.MigrateConnectors(context.Background())
	assert.NoError(err)
}
