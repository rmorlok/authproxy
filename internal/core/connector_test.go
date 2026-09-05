package core

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/encrypt"
	encryptmock "github.com/rmorlok/authproxy/internal/encrypt/mock"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
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
	c := wrapConnector(dbConnector, s)

	// Verify
	assert.Equal(t, dbConnector, c.ConnectorWithDefinition)
	assert.Equal(t, s, c.s)
	assert.Nil(t, c.def)
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

	c := wrapConnector(dbConnector, s)

	// Create a connector definition
	def := &cschema.ConnectorDefinition{
		DisplayName: "Test Connector",
		Description: "A test connector",
	}
	defJSON, _ := json.Marshal(def)

	// Set up expectations for the encrypt service
	mockEncrypt.EXPECT().
		DecryptString(gomock.Any(), encfield.EncryptedField{ID: "dek_test", Data: "encrypted-data"}).
		Return(string(defJSON), nil)

	// Test
	result := c.GetDefinition()

	// Verify
	assert.Equal(t, def.DisplayName, result.DisplayName)
	assert.Equal(t, def.Description, result.Description)

	// Test caching - should not call decrypt again
	result2 := c.GetDefinition()
	assert.Equal(t, result, result2)
}

func TestConnector_GetResource(t *testing.T) {
	createdAt := time.Date(2026, time.September, 5, 14, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Minute)
	id := apid.MustParse("cxr_testresource00001")
	definition := &cschema.ConnectorDefinition{
		DisplayName: "Test Connector",
		Description: "A test connector",
	}
	labels := database.Labels{"team": "platform"}
	annotations := database.Annotations{"example.com/owner": "integrations"}
	c := &Connector{
		ConnectorWithDefinition: database.ConnectorWithDefinition{
			Id:          id,
			Name:        "test-connector",
			Namespace:   "root.acme",
			Version:     3,
			State:       database.ConnectorDefinitionVersionStateArchived,
			Labels:      labels,
			Annotations: annotations,
			CreatedAt:   createdAt,
			UpdatedAt:   updatedAt,
		},
		def: definition,
	}

	resource := c.GetResource()
	require.Equal(t, cschema.ConnectorKind, resource.Kind)
	require.Equal(t, id.String(), resource.Metadata.ID)
	require.Equal(t, "test-connector", string(resource.Metadata.Name))
	require.Equal(t, "root.acme", resource.Metadata.Namespace)
	require.Equal(t, uint64(3), resource.Metadata.Generation)
	require.Equal(t, createdAt, *resource.Metadata.CreatedAt)
	require.Equal(t, updatedAt, *resource.Metadata.UpdatedAt)
	require.Equal(t, cschema.ConnectorReleaseStatePrimary, resource.Spec.Release.DesiredState)
	require.Equal(t, cschema.ConnectorReleaseStateArchived, resource.Status.Release.State)
	require.Equal(t, definition, &resource.Spec.Definition)
	require.NoError(t, resource.ValidateFor(meta.ValidationModeResponse, nil))

	resource.Metadata.Labels["team"] = "changed"
	resource.Metadata.Annotations["example.com/owner"] = "changed"
	resource.Spec.Definition.Description = "changed"
	require.Equal(t, "platform", labels["team"])
	require.Equal(t, "integrations", annotations["example.com/owner"])
	require.Equal(t, "A test connector", definition.Description)
}

func TestConnector_GetHashDerivesFromEncryptedDefinition(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEncrypt := encryptmock.NewMockE(ctrl)
	encryptedDefinition := encfield.EncryptedField{ID: "dek_test", Data: "encrypted-data"}
	def := &cschema.ConnectorDefinition{
		DisplayName: "Test Connector",
	}
	defJSON := util.Must(json.Marshal(def))
	mockEncrypt.EXPECT().
		DecryptString(gomock.Any(), encryptedDefinition).
		Return(string(defJSON), nil)

	c := wrapConnector(database.ConnectorWithDefinition{
		Id:                  apid.New(apid.PrefixConnectorVersion),
		Version:             1,
		EncryptedDefinition: encryptedDefinition,
	}, &service{encrypt: mockEncrypt, logger: aplog.NewNoopLogger()})

	require.Equal(t, def.Hash(), c.GetHash())
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

	c := wrapConnector(dbConnector, s)

	// Create a connector definition
	def := &cschema.ConnectorDefinition{
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
			`{"auth":null,"description":"A test connector","displayName":"Test Connector","logo":null}`).
		Return(newEncryptedDef, nil)

	// Test
	err := c.setDefinition(def)

	// Verify
	assert.NoError(t, err)
	assert.Equal(t, expectedHash, c.Hash)
	assert.Equal(t, newEncryptedDef, c.EncryptedDefinition)
	assert.Equal(t, def, c.def)
}

func TestConnector_SetDefinitionResetsJavascriptLibrary(t *testing.T) {
	c := NewTestConnector(cschema.ConnectorDefinition{
		Javascript: `function isUpdated() { return false; }`,
	})

	library, err := c.getJavascriptLibrary()
	require.NoError(t, err)
	ok, err := library.NewContext(nil).EvaluateBoolean(`isUpdated()`)
	require.NoError(t, err)
	assert.False(t, ok)

	err = c.setDefinition(&cschema.ConnectorDefinition{
		Javascript: `function isUpdated() { return true; }`,
	})
	require.NoError(t, err)

	library, err = c.getJavascriptLibrary()
	require.NoError(t, err)
	ok, err = library.NewContext(nil).EvaluateBoolean(`isUpdated()`)
	require.NoError(t, err)
	assert.True(t, ok)
}

// NewTestConnector creates a hydrated test connector using the provided definition.
func NewTestConnector(c cschema.ConnectorDefinition) *Connector {
	e := encrypt.NewFakeEncryptService(false)
	connectorId := apid.New(apid.PrefixActor)
	encryptedDefinition, err := e.EncryptStringForEntity(context.Background(), &namespaceHolder{namespace: "root"}, string(util.Must(json.Marshal(c))))
	if err != nil {
		panic(err)
	}

	dbConnector := database.ConnectorWithDefinition{
		Id:                  connectorId,
		Version:             1,
		Labels:              map[string]string{"type": "test-connector"},
		State:               database.ConnectorDefinitionVersionStatePrimary,
		EncryptedDefinition: encryptedDefinition,
	}

	return wrapConnector(dbConnector, &service{encrypt: e, logger: aplog.NewNoopLogger()})
}
