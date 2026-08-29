package core

import (
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encfield"
	encryptmock "github.com/rmorlok/authproxy/internal/encrypt/mock"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
)

func testConfiguredConnectorResource(id apid.ID) *cschema.Connector {
	return &cschema.Connector{
		TypeMeta: meta.NewTypeMeta(cschema.ConnectorKind),
		Metadata: meta.ObjectMeta{
			ID:          id.String(),
			Name:        "test-connector",
			Namespace:   "root.test",
			Generation:  3,
			Labels:      map[string]string{"type": "test"},
			Annotations: map[string]string{"example.com/owner": "platform"},
		},
		Spec: cschema.ConnectorSpec{
			Release: cschema.ConnectorReleaseSpec{DesiredState: cschema.ConnectorReleaseStateDraft},
			Definition: cschema.ConnectorDefinition{
				DisplayName: "Test Connector",
				Description: "A test connector",
			},
		},
	}
}

func TestConnectorBuilderWithConfigSeparatesResourceFields(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockEncrypt := encryptmock.NewMockE(ctrl)
	s := &service{encrypt: mockEncrypt, logger: aplog.NewNoopLogger()}
	id := apid.New(apid.PrefixConnector)
	resource := testConfiguredConnectorResource(id)

	mockEncrypt.EXPECT().
		EncryptStringForEntity(gomock.Any(), gomock.Any(), `{"auth":null,"description":"A test connector","displayName":"Test Connector","logo":null}`).
		Return(encfield.EncryptedField{ID: "dek_test", Data: "encrypted-data"}, nil)

	connector, err := newConnectorBuilder(s).WithConfig(resource).Build()
	require.NoError(t, err)
	require.Equal(t, id, connector.Id)
	require.Equal(t, uint64(3), connector.Version)
	require.Equal(t, "root.test", connector.Namespace)
	require.Equal(t, resource.Metadata.Name, connector.Name)
	require.Equal(t, database.Labels(resource.Metadata.Labels), connector.Labels)
	require.Equal(t, database.Annotations(resource.Metadata.Annotations), connector.Annotations)
	require.Equal(t, resource.Spec.Definition.Hash(), connector.Hash)
}

func TestConnectorBuilderSettersUpdateResourceMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockEncrypt := encryptmock.NewMockE(ctrl)
	mockEncrypt.EXPECT().EncryptStringForEntity(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(encfield.EncryptedField{ID: "dek_test", Data: "encrypted-data"}, nil)

	id := apid.New(apid.PrefixConnector)
	resource := testConfiguredConnectorResource(apid.Nil)
	connector, err := newConnectorBuilder(&service{encrypt: mockEncrypt}).WithConfig(resource).
		WithId(id).
		WithVersion(7).
		WithState("primary").
		Build()
	require.NoError(t, err)

	require.Equal(t, id.String(), resource.Metadata.ID)
	require.Equal(t, uint64(7), resource.Metadata.Generation)
	require.Equal(t, cschema.ConnectorReleaseStatePrimary, resource.Spec.Release.DesiredState)
	require.Equal(t, id, connector.Id)
	require.Equal(t, uint64(7), connector.Version)
	require.Equal(t, database.ConnectorDefinitionVersionStatePrimary, connector.State)
}

func TestConnectorBuilderBuildWithDefinition(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockEncrypt := encryptmock.NewMockE(ctrl)
	s := &service{encrypt: mockEncrypt, logger: aplog.NewNoopLogger()}
	definition := &cschema.ConnectorDefinition{DisplayName: "API-created"}
	id := apid.New(apid.PrefixConnector)

	mockEncrypt.EXPECT().EncryptStringForEntity(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(encfield.EncryptedField{ID: "dek_test", Data: "encrypted-data"}, nil)
	connector, err := newConnectorBuilder(s).
		WithDefinition(definition).
		WithId(id).
		WithVersion(1).
		WithState("draft").
		Build()
	require.NoError(t, err)
	require.Equal(t, id, connector.Id)
	require.Equal(t, definition.Hash(), connector.Hash)
}

func TestConnectorBuilderBuildErrors(t *testing.T) {
	connector, err := newConnectorBuilder(&service{}).Build()
	require.ErrorIs(t, err, errNilConnector)
	require.Nil(t, connector)

	ctrl := gomock.NewController(t)
	mockEncrypt := encryptmock.NewMockE(ctrl)
	mockEncrypt.EXPECT().EncryptStringForEntity(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(encfield.EncryptedField{}, errors.New("encryption error"))
	connector, err = newConnectorBuilder(&service{encrypt: mockEncrypt}).
		WithDefinition(&cschema.ConnectorDefinition{}).
		Build()
	require.ErrorContains(t, err, "encryption error")
	require.Nil(t, connector)
}
