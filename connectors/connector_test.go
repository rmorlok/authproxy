package connectors

import (
	"encoding/json"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/config/connectors"
	"github.com/rmorlok/authproxy/database"
	encryptmock "github.com/rmorlok/authproxy/encrypt/mock"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestWrapConnectorVersion(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEncrypt := encryptmock.NewMockE(ctrl)
	s := &service{
		encrypt: mockEncrypt,
	}

	connectorID := uuid.New()
	dbConnectorVersion := database.ConnectorVersion{
		ID:                  connectorID,
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
	}

	connectorID := uuid.New()
	dbConnectorVersion := database.ConnectorVersion{
		ID:                  connectorID,
		Version:             1,
		Type:                "test-connector",
		State:               database.ConnectorVersionStateDraft,
		Hash:                "some-hash",
		EncryptedDefinition: "encrypted-data",
	}

	cv := wrapConnectorVersion(dbConnectorVersion, s)

	// Create a connector definition
	def := &connectors.Connector{
		Type:        "test-connector",
		DisplayName: "Test Connector",
		Description: "A test connector",
	}
	defJSON, _ := json.Marshal(def)

	// Set up expectations for the encrypt service
	mockEncrypt.EXPECT().
		DecryptStringForConnector(gomock.Any(), cv.ConnectorVersion, "encrypted-data").
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
	}

	connectorID := uuid.New()
	dbConnectorVersion := database.ConnectorVersion{
		ID:                  connectorID,
		Version:             1,
		Type:                "test-connector",
		State:               database.ConnectorVersionStateDraft,
		Hash:                "some-hash",
		EncryptedDefinition: "encrypted-data",
	}

	cv := wrapConnectorVersion(dbConnectorVersion, s)

	// Create a connector definition
	def := &connectors.Connector{
		Type:        "test-connector",
		DisplayName: "Test Connector",
		Description: "A test connector",
	}

	// Calculate expected hash
	expectedHash := def.Hash()

	// Set up expectations for the encrypt service
	mockEncrypt.EXPECT().
		EncryptStringForConnector(gomock.Any(), cv.ConnectorVersion, gomock.Any()).
		Return("new-encrypted-data", nil)

	// Test
	err := cv.setDefinition(def)

	// Verify
	assert.NoError(t, err)
	assert.Equal(t, expectedHash, cv.Hash)
	assert.Equal(t, "new-encrypted-data", cv.EncryptedDefinition)
	assert.Equal(t, def, cv.def)
}
