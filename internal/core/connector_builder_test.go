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
	"github.com/stretchr/testify/require"
)

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
		WithGeneration(1).
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
