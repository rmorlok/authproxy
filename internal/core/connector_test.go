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
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type namespaceHolder struct {
	namespace string
}

func (n *namespaceHolder) GetNamespace() string {
	return n.namespace
}

func TestWrapConnector(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEncrypt := encryptmock.NewMockE(ctrl)
	s := &service{
		encrypt: mockEncrypt,
		logger:  aplog.NewNoopLogger(),
	}

	connectorId := apid.New(apid.PrefixActor)
	dbConnector := database.ConnectorWithDefinition{
		Id:                  connectorId,
		Version:             1,
		Labels:              map[string]string{"type": "test-connector"},
		State:               database.ConnectorDefinitionVersionStateDraft,
		EncryptedDefinition: encfield.EncryptedField{ID: "dek_test", Data: "encrypted-data"},
	}

	// Test
	cv := wrapConnector(dbConnector, s)

	// Verify
	assert.Equal(t, dbConnector, cv.ConnectorWithDefinition)
	assert.Equal(t, s, cv.s)
	assert.Nil(t, cv.def)
}

func TestConnector_GetDefinition(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEncrypt := encryptmock.NewMockE(ctrl)
	s := &service{
		encrypt: mockEncrypt,
		logger:  aplog.NewNoopLogger(),
	}

	connectorId := apid.New(apid.PrefixActor)
	dbConnector := database.ConnectorWithDefinition{
		Id:                  connectorId,
		Version:             1,
		Labels:              map[string]string{"type": "test-connector"},
		State:               database.ConnectorDefinitionVersionStateDraft,
		EncryptedDefinition: encfield.EncryptedField{ID: "dek_test", Data: "encrypted-data"},
	}

	cv := wrapConnector(dbConnector, s)

	// Create a connector definition
	def := &cschema.Connector{
		Labels:      map[string]string{"type": "test-connector"},
		DisplayName: "Test Connector",
		Description: "A test connector",
	}
	defJSON, _ := json.Marshal(def)

	// Set up expectations for the encrypt service
	mockEncrypt.EXPECT().
		DecryptString(gomock.Any(), encfield.EncryptedField{ID: "dek_test", Data: "encrypted-data"}).
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

func TestConnector_GetHashDerivesFromEncryptedDefinition(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEncrypt := encryptmock.NewMockE(ctrl)
	encryptedDefinition := encfield.EncryptedField{ID: "dek_test", Data: "encrypted-data"}
	def := &cschema.Connector{
		Labels:      map[string]string{"type": "test-connector"},
		DisplayName: "Test Connector",
	}
	defJSON := util.Must(json.Marshal(def))
	mockEncrypt.EXPECT().
		DecryptString(gomock.Any(), encryptedDefinition).
		Return(string(defJSON), nil)

	cv := wrapConnector(database.ConnectorWithDefinition{
		Id:                  apid.New(apid.PrefixConnectorVersion),
		Version:             1,
		EncryptedDefinition: encryptedDefinition,
	}, &service{encrypt: mockEncrypt, logger: aplog.NewNoopLogger()})

	require.Equal(t, def.Hash(), cv.GetHash())
}

func TestConnector_SetDefinition(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEncrypt := encryptmock.NewMockE(ctrl)
	s := &service{
		encrypt: mockEncrypt,
		logger:  aplog.NewNoopLogger(),
	}

	connectorId := apid.New(apid.PrefixActor)
	dbConnector := database.ConnectorWithDefinition{
		Id:                  connectorId,
		Version:             1,
		Labels:              map[string]string{"type": "test-connector"},
		State:               database.ConnectorDefinitionVersionStateDraft,
		EncryptedDefinition: encfield.EncryptedField{ID: "dek_test", Data: "encrypted-data"},
	}

	cv := wrapConnector(dbConnector, s)

	// Create a connector definition
	def := &cschema.Connector{
		Labels:      map[string]string{"type": "test-connector"},
		DisplayName: "Test Connector",
		Description: "A test connector",
	}

	// Calculate expected hash
	expectedHash := def.Hash()

	// Set up expectations for the encrypt service
	newEncryptedDef := encfield.EncryptedField{ID: "dek_test", Data: "new-encrypted-data"}
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

func TestConnector_SetDefinitionResetsJavascriptLibrary(t *testing.T) {
	cv := NewTestConnector(cschema.Connector{
		Javascript: `function isUpdated() { return false; }`,
	})

	library, err := cv.getJavascriptLibrary()
	require.NoError(t, err)
	ok, err := library.NewContext(nil).EvaluateBoolean(`isUpdated()`)
	require.NoError(t, err)
	assert.False(t, ok)

	err = cv.setDefinition(&cschema.Connector{
		Javascript: `function isUpdated() { return true; }`,
	})
	require.NoError(t, err)

	library, err = cv.getJavascriptLibrary()
	require.NoError(t, err)
	ok, err = library.NewContext(nil).EvaluateBoolean(`isUpdated()`)
	require.NoError(t, err)
	assert.True(t, ok)
}

// NewTestConnector creates a hydrated test connector using the provided definition.
func NewTestConnector(c cschema.Connector) *Connector {
	e := encrypt.NewFakeEncryptService(false)
	connectorId := apid.New(apid.PrefixActor)
	if c.Id != apid.Nil {
		connectorId = c.Id
	}
	version := uint64(1)
	if c.Version != 0 {
		version = c.Version
	}
	state := database.ConnectorDefinitionVersionStatePrimary
	if c.State != "" {
		state = database.ConnectorDefinitionVersionState(c.State)
	}
	encryptedDefinition, err := e.EncryptStringForEntity(context.Background(), &namespaceHolder{namespace: "root"}, string(util.Must(json.Marshal(c))))
	if err != nil {
		panic(err)
	}

	dbConnector := database.ConnectorWithDefinition{
		Id:                  connectorId,
		Version:             version,
		Labels:              map[string]string{"type": "test-connector"},
		State:               state,
		EncryptedDefinition: encryptedDefinition,
	}

	return wrapConnector(dbConnector, &service{encrypt: e, logger: aplog.NewNoopLogger()})
}
