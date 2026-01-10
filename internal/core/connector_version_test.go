package core

import (
	"encoding/json"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/aplog"
	coreMock "github.com/rmorlok/authproxy/internal/core/mock"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	encryptmock "github.com/rmorlok/authproxy/internal/encrypt/mock"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/stretchr/testify/assert"
)

func TestWrapConnectorVersion(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEncrypt := encryptmock.NewMockE(ctrl)
	s := &service{
		encrypt: mockEncrypt,
		logger:  aplog.NewNoopLogger(),
	}

	connectorId := uuid.New()
	dbConnectorVersion := database.ConnectorVersion{
		Id:                  connectorId,
		Version:             1,
		Type:                "test-connector",
		State:               database.ConnectorVersionStateDraft,
		Hash:                "some-hash",
		EncryptedDefinition: "encrypted-data",
	}

	// Test
	cv := wrapConnectorVersion(dbConnectorVersion, s)

	// Verify
	assert.Equal(t, dbConnectorVersion, cv.ConnectorVersion)
	assert.Equal(t, s, cv.s)
	assert.Nil(t, cv.def)
}

func TestConnectorVersion_GetDefinition(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEncrypt := encryptmock.NewMockE(ctrl)
	s := &service{
		encrypt: mockEncrypt,
		logger:  aplog.NewNoopLogger(),
	}

	connectorId := uuid.New()
	dbConnectorVersion := database.ConnectorVersion{
		Id:                  connectorId,
		Version:             1,
		Type:                "test-connector",
		State:               database.ConnectorVersionStateDraft,
		Hash:                "some-hash",
		EncryptedDefinition: "encrypted-data",
	}

	cv := wrapConnectorVersion(dbConnectorVersion, s)

	// Create a connector definition
	def := &cschema.Connector{
		Type:        "test-connector",
		DisplayName: "Test Connector",
		Description: "A test connector",
	}
	defJSON, _ := json.Marshal(def)

	// Set up expectations for the encrypt service
	mockEncrypt.EXPECT().
		DecryptStringForConnector(gomock.Any(), cv, "encrypted-data").
		Return(string(defJSON), nil)

	// Test
	result := cv.GetDefinition()

	// Verify
	assert.Equal(t, def.Type, result.Type)
	assert.Equal(t, def.DisplayName, result.DisplayName)
	assert.Equal(t, def.Description, result.Description)

	// Test caching - should not call decrypt again
	result2 := cv.GetDefinition()
	assert.Equal(t, result, result2)
}

func TestConnectorVersion_SetDefinition(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEncrypt := encryptmock.NewMockE(ctrl)
	s := &service{
		encrypt: mockEncrypt,
		logger:  aplog.NewNoopLogger(),
	}

	connectorId := uuid.New()
	dbConnectorVersion := database.ConnectorVersion{
		Id:                  connectorId,
		Version:             1,
		Type:                "test-connector",
		State:               database.ConnectorVersionStateDraft,
		Hash:                "some-hash",
		EncryptedDefinition: "encrypted-data",
	}

	cv := wrapConnectorVersion(dbConnectorVersion, s)

	// Create a connector definition
	def := &cschema.Connector{
		Type:        "test-connector",
		DisplayName: "Test Connector",
		Description: "A test connector",
	}

	// Calculate expected hash
	expectedHash := def.Hash()

	// Set up expectations for the encrypt service
	mockEncrypt.EXPECT().
		EncryptStringForConnector(
			gomock.Any(),
			coreMock.ConnectorVersionMatcher{
				ExpectedId:      cv.ConnectorVersion.Id,
				ExpectedVersion: cv.ConnectorVersion.Version,
			},
			gomock.Any()).
		Return("new-encrypted-data", nil)

	// Test
	err := cv.setDefinition(def)

	// Verify
	assert.NoError(t, err)
	assert.Equal(t, expectedHash, cv.Hash)
	assert.Equal(t, "new-encrypted-data", cv.EncryptedDefinition)
	assert.Equal(t, def, cv.def)
}

// NewTestConnectorVersion creates a new test connector version using provided connector configuration data.
func NewTestConnectorVersion(c cschema.Connector) *ConnectorVersion {
	e := encrypt.NewFakeEncryptService(false)
	connectorId := uuid.New()
	if c.Id != uuid.Nil {
		connectorId = c.Id
	}
	version := uint64(1)
	if c.Version != 0 {
		version = c.Version
	}
	t := "test-connector"
	if c.Type != "" {
		t = c.Type
	}
	state := database.ConnectorVersionStatePrimary
	if c.State != "" {
		state = database.ConnectorVersionState(c.State)
	}
	encryptedDefinition, err := json.Marshal(c)
	if err != nil {
		panic(err)
	}

	dbConnectorVersion := database.ConnectorVersion{
		Id:                  connectorId,
		Version:             version,
		Type:                t,
		State:               state,
		Hash:                "some-hash",
		EncryptedDefinition: string(encryptedDefinition),
	}

	return wrapConnectorVersion(dbConnectorVersion, &service{encrypt: e, logger: aplog.NewNoopLogger()})
}
