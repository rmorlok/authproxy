package core

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/encrypt"
	encryptmock "github.com/rmorlok/authproxy/internal/encrypt/mock"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/assert"
)

type namespaceHolder struct {
	namespace string
}

func (n *namespaceHolder) GetNamespace() string {
	return n.namespace
}

func TestWrapConnectorVersion(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEncrypt := encryptmock.NewMockE(ctrl)
	s := &service{
		encrypt: mockEncrypt,
		logger:  aplog.NewNoopLogger(),
	}

	connectorId := apid.New(apid.PrefixActor)
	dbConnectorVersion := database.ConnectorVersion{
		Id:                  connectorId,
		Version:             1,
		Labels:              map[string]string{"type": "test-connector"},
		State:               database.ConnectorVersionStateDraft,
		Hash:                "some-hash",
		EncryptedDefinition: encfield.EncryptedField{ID: "ekv_test", Data: "encrypted-data"},
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

	connectorId := apid.New(apid.PrefixActor)
	dbConnectorVersion := database.ConnectorVersion{
		Id:                  connectorId,
		Version:             1,
		Labels:              map[string]string{"type": "test-connector"},
		State:               database.ConnectorVersionStateDraft,
		Hash:                "some-hash",
		EncryptedDefinition: encfield.EncryptedField{ID: "ekv_test", Data: "encrypted-data"},
	}

	cv := wrapConnectorVersion(dbConnectorVersion, s)

	// Create a connector definition
	def := &cschema.Connector{
		Labels:      map[string]string{"type": "test-connector"},
		DisplayName: "Test Connector",
		Description: "A test connector",
	}
	defJSON, _ := json.Marshal(def)

	// Set up expectations for the encrypt service
	mockEncrypt.EXPECT().
		DecryptString(gomock.Any(), encfield.EncryptedField{ID: "ekv_test", Data: "encrypted-data"}).
		Return(string(defJSON), nil)

	// Test
	result := cv.GetDefinition()

	// Verify
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

	connectorId := apid.New(apid.PrefixActor)
	dbConnectorVersion := database.ConnectorVersion{
		Id:                  connectorId,
		Version:             1,
		Labels:              map[string]string{"type": "test-connector"},
		State:               database.ConnectorVersionStateDraft,
		Hash:                "some-hash",
		EncryptedDefinition: encfield.EncryptedField{ID: "ekv_test", Data: "encrypted-data"},
	}

	cv := wrapConnectorVersion(dbConnectorVersion, s)

	// Create a connector definition
	def := &cschema.Connector{
		Labels:      map[string]string{"type": "test-connector"},
		DisplayName: "Test Connector",
		Description: "A test connector",
	}

	// Calculate expected hash
	expectedHash := def.Hash()

	// Set up expectations for the encrypt service
	newEncryptedDef := encfield.EncryptedField{ID: "ekv_test", Data: "new-encrypted-data"}
	mockEncrypt.EXPECT().
		EncryptStringForEntity(
			gomock.Any(),
			gomock.Any(),
			gomock.Any()).
		Return(newEncryptedDef, nil)

	// Test
	err := cv.setDefinition(def)

	// Verify
	assert.NoError(t, err)
	assert.Equal(t, expectedHash, cv.Hash)
	assert.Equal(t, newEncryptedDef, cv.EncryptedDefinition)
	assert.Equal(t, def, cv.def)
}

// NewTestConnectorVersion creates a new test connector version using provided connector configuration data.
func NewTestConnectorVersion(c cschema.Connector) *ConnectorVersion {
	e := encrypt.NewFakeEncryptService(false)
	connectorId := apid.New(apid.PrefixActor)
	if c.Id != apid.Nil {
		connectorId = c.Id
	}
	version := uint64(1)
	if c.Version != 0 {
		version = c.Version
	}
	state := database.ConnectorVersionStatePrimary
	if c.State != "" {
		state = database.ConnectorVersionState(c.State)
	}
	encryptedDefinition, err := e.EncryptStringForEntity(context.Background(), &namespaceHolder{namespace: "root"}, string(util.Must(json.Marshal(c))))
	if err != nil {
		panic(err)
	}

	dbConnectorVersion := database.ConnectorVersion{
		Id:                  connectorId,
		Version:             version,
		Labels:              map[string]string{"type": "test-connector"},
		State:               state,
		Hash:                "some-hash",
		EncryptedDefinition: encryptedDefinition,
	}

	return wrapConnectorVersion(dbConnectorVersion, &service{encrypt: e, logger: aplog.NewNoopLogger()})
}
