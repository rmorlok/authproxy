package core

import (
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/encfield"
	encryptmock "github.com/rmorlok/authproxy/internal/encrypt/mock"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/assert"
)

func TestNewVersionBuilder(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEncrypt := encryptmock.NewMockE(ctrl)
	s := &service{
		encrypt: mockEncrypt,
		logger:  aplog.NewNoopLogger(),
	}

	// Test
	builder := newConnectorBuilder(s)

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
		logger:  aplog.NewNoopLogger(),
	}

	builder := newConnectorBuilder(s)

	// Create a test configuration
	connectorID := apid.New(apid.PrefixActor)
	c := &cschema.Connector{
		Id:          connectorID,
		Version:     1,
		Labels:      map[string]string{"type": "test-connector"},
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
	connector := &Connector{}
	builder.versionSetters[0](connector)
	assert.Equal(t, uint64(1), connector.Version)
	assert.Equal(t, connectorID, connector.Id)
}

func TestVersionBuilder_WithId(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEncrypt := encryptmock.NewMockE(ctrl)
	s := &service{
		encrypt: mockEncrypt,
		logger:  aplog.NewNoopLogger(),
	}

	builder := newConnectorBuilder(s)

	// Create a test ID
	connectorID := apid.New(apid.PrefixActor)

	// Test
	result := builder.WithId(connectorID)

	// Verify
	assert.Equal(t, builder, result, "WithId should return the builder for chaining")
	assert.NotEmpty(t, builder.versionSetters)
	assert.NotEmpty(t, builder.configSetters)

	// Test the version setter function
	connector := &Connector{}
	builder.versionSetters[0](connector)
	assert.Equal(t, connectorID, connector.Id)

	// Test the config setter function
	c := &cschema.Connector{}
	builder.configSetters[0](c)
	assert.Equal(t, connectorID, c.Id)
}

func TestVersionBuilder_WithVersion(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEncrypt := encryptmock.NewMockE(ctrl)
	s := &service{
		encrypt: mockEncrypt,
		logger:  aplog.NewNoopLogger(),
	}

	builder := newConnectorBuilder(s)

	// Create a test version
	version := uint64(2)

	// Test
	result := builder.WithVersion(version)

	// Verify
	assert.Equal(t, builder, result, "WithVersion should return the builder for chaining")
	assert.NotEmpty(t, builder.versionSetters)
	assert.NotEmpty(t, builder.configSetters)

	// Test the version setter function
	connector := &Connector{}
	builder.versionSetters[0](connector)
	assert.Equal(t, version, connector.Version)

	// Test the config setter function
	c := &cschema.Connector{}
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
		logger:  aplog.NewNoopLogger(),
	}

	builder := newConnectorBuilder(s)

	// Create a test configuration
	connectorID := apid.New(apid.PrefixActor)
	c := &cschema.Connector{
		Id:          connectorID,
		Version:     1,
		Labels:      map[string]string{"type": "test-connector"},
		DisplayName: "Test Connector",
		Description: "A test connector",
	}

	builder.WithConfig(c)

	// Set up expectations for the encrypt service
	mockEncrypt.EXPECT().
		EncryptStringForEntity(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(encfield.EncryptedField{ID: "dek_test", Data: "encrypted-data"}, nil)

	// Test
	connector, err := builder.Build()

	// Verify
	assert.NoError(t, err)
	assert.NotNil(t, connector)
	assert.Equal(t, connectorID, connector.Id)
	assert.Equal(t, uint64(1), connector.Version)
	assert.Equal(t, c.Hash(), connector.Hash)
	assert.Equal(t, encfield.EncryptedField{ID: "dek_test", Data: "encrypted-data"}, connector.EncryptedDefinition)
}

func TestVersionBuilder_Build_NilConnector(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEncrypt := encryptmock.NewMockE(ctrl)
	s := &service{
		encrypt: mockEncrypt,
		logger:  aplog.NewNoopLogger(),
	}

	builder := newConnectorBuilder(s)

	// Test
	connector, err := builder.Build()

	// Verify
	assert.Error(t, err)
	assert.Equal(t, errNilConnector, err)
	assert.Nil(t, connector)
}

func TestVersionBuilder_Build_EncryptError(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEncrypt := encryptmock.NewMockE(ctrl)
	s := &service{
		encrypt: mockEncrypt,
		logger:  aplog.NewNoopLogger(),
	}

	builder := newConnectorBuilder(s)

	// Create a test configuration
	connectorID := apid.New(apid.PrefixActor)
	c := &cschema.Connector{
		Id:          connectorID,
		Version:     1,
		Labels:      map[string]string{"type": "test-connector"},
		DisplayName: "Test Connector",
		Description: "A test connector",
	}

	builder.WithConfig(c)

	// Set up expectations for the encrypt service with error
	mockEncrypt.EXPECT().
		EncryptStringForEntity(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(encfield.EncryptedField{}, errors.New("encryption error"))

	// Test
	connector, err := builder.Build()

	// Verify
	assert.Error(t, err)
	assert.Nil(t, connector)
	assert.Contains(t, err.Error(), "encryption error")
}
