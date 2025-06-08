package connectors

import (
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/config"
	encryptmock "github.com/rmorlok/authproxy/encrypt/mock"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewVersionBuilder(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEncrypt := encryptmock.NewMockE(ctrl)
	s := &service{
		encrypt: mockEncrypt,
	}

	// Test
	builder := newVersionBuilder(s)
	
	// Verify
	assert.NotNil(t, builder)
	assert.Equal(t, s, builder.s)
	assert.Nil(t, builder.c)
	assert.Empty(t, builder.configSetters)
	assert.Empty(t, builder.versionSetters)
}

func TestVersionBuilder_WithConfig(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEncrypt := encryptmock.NewMockE(ctrl)
	s := &service{
		encrypt: mockEncrypt,
	}

	builder := newVersionBuilder(s)
	
	// Create a test configuration
	connectorID := uuid.New()
	c := &config.Connector{
		Id:          connectorID,
		Version:     1,
		Type:        "test-connector",
		DisplayName: "Test Connector",
		Description: "A test connector",
	}

	// Test
	result := builder.WithConfig(c)
	
	// Verify
	assert.Equal(t, builder, result, "WithConfig should return the builder for chaining")
	assert.Equal(t, c, builder.c)
	assert.NotEmpty(t, builder.versionSetters)
	
	// Test the setter function
	cv := &ConnectorVersion{}
	builder.versionSetters[0](cv)
	assert.Equal(t, int64(1), cv.Version)
	assert.Equal(t, "test-connector", cv.Type)
	assert.Equal(t, connectorID, cv.ID)
}

func TestVersionBuilder_WithId(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEncrypt := encryptmock.NewMockE(ctrl)
	s := &service{
		encrypt: mockEncrypt,
	}

	builder := newVersionBuilder(s)
	
	// Create a test ID
	connectorID := uuid.New()

	// Test
	result := builder.WithId(connectorID)
	
	// Verify
	assert.Equal(t, builder, result, "WithId should return the builder for chaining")
	assert.NotEmpty(t, builder.versionSetters)
	assert.NotEmpty(t, builder.configSetters)
	
	// Test the version setter function
	cv := &ConnectorVersion{}
	builder.versionSetters[0](cv)
	assert.Equal(t, connectorID, cv.ID)
	
	// Test the config setter function
	c := &config.Connector{}
	builder.configSetters[0](c)
	assert.Equal(t, connectorID, c.Id)
}

func TestVersionBuilder_WithType(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEncrypt := encryptmock.NewMockE(ctrl)
	s := &service{
		encrypt: mockEncrypt,
	}

	builder := newVersionBuilder(s)
	
	// Create a test type
	connectorType := "test-connector"

	// Test
	result := builder.WithType(connectorType)
	
	// Verify
	assert.Equal(t, builder, result, "WithType should return the builder for chaining")
	assert.NotEmpty(t, builder.versionSetters)
	assert.NotEmpty(t, builder.configSetters)
	
	// Test the version setter function
	cv := &ConnectorVersion{}
	builder.versionSetters[0](cv)
	assert.Equal(t, connectorType, cv.Type)
	
	// Test the config setter function
	c := &config.Connector{}
	builder.configSetters[0](c)
	assert.Equal(t, connectorType, c.Type)
}

func TestVersionBuilder_WithVersion(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEncrypt := encryptmock.NewMockE(ctrl)
	s := &service{
		encrypt: mockEncrypt,
	}

	builder := newVersionBuilder(s)
	
	// Create a test version
	version := int64(2)

	// Test
	result := builder.WithVersion(version)
	
	// Verify
	assert.Equal(t, builder, result, "WithVersion should return the builder for chaining")
	assert.NotEmpty(t, builder.versionSetters)
	assert.NotEmpty(t, builder.configSetters)
	
	// Test the version setter function
	cv := &ConnectorVersion{}
	builder.versionSetters[0](cv)
	assert.Equal(t, version, cv.Version)
	
	// Test the config setter function
	c := &config.Connector{}
	builder.configSetters[0](c)
	assert.Equal(t, uint64(version), c.Version)
}

func TestVersionBuilder_Build_Success(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEncrypt := encryptmock.NewMockE(ctrl)
	s := &service{
		encrypt: mockEncrypt,
	}

	builder := newVersionBuilder(s)
	
	// Create a test configuration
	connectorID := uuid.New()
	c := &config.Connector{
		Id:          connectorID,
		Version:     1,
		Type:        "test-connector",
		DisplayName: "Test Connector",
		Description: "A test connector",
	}
	
	builder.WithConfig(c)
	
	// Set up expectations for the encrypt service
	mockEncrypt.EXPECT().
		EncryptForConnector(gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]byte("encrypted-data"), nil)

	// Test
	cv, err := builder.Build()
	
	// Verify
	assert.NoError(t, err)
	assert.NotNil(t, cv)
	assert.Equal(t, connectorID, cv.ID)
	assert.Equal(t, int64(1), cv.Version)
	assert.Equal(t, "test-connector", cv.Type)
	assert.Equal(t, c.Hash(), cv.Hash)
	assert.Equal(t, "encrypted-data", cv.EncryptedDefinition)
}

func TestVersionBuilder_Build_NilConnector(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEncrypt := encryptmock.NewMockE(ctrl)
	s := &service{
		encrypt: mockEncrypt,
	}

	builder := newVersionBuilder(s)
	
	// Test
	cv, err := builder.Build()
	
	// Verify
	assert.Error(t, err)
	assert.Equal(t, errNilConnector, err)
	assert.Nil(t, cv)
}

func TestVersionBuilder_Build_EncryptError(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEncrypt := encryptmock.NewMockE(ctrl)
	s := &service{
		encrypt: mockEncrypt,
	}

	builder := newVersionBuilder(s)
	
	// Create a test configuration
	connectorID := uuid.New()
	c := &config.Connector{
		Id:          connectorID,
		Version:     1,
		Type:        "test-connector",
		DisplayName: "Test Connector",
		Description: "A test connector",
	}
	
	builder.WithConfig(c)
	
	// Set up expectations for the encrypt service with error
	mockEncrypt.EXPECT().
		EncryptForConnector(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New("encryption error"))

	// Test
	cv, err := builder.Build()
	
	// Verify
	assert.Error(t, err)
	assert.Nil(t, cv)
	assert.Contains(t, err.Error(), "encryption error")
}