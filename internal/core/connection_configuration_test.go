package core

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/encrypt"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestConnectionWithService(s *service) *connection {
	cv := NewTestConnectorVersion(cschema.Connector{})
	connId := apid.New(apid.PrefixConnection)
	return &connection{
		Connection: database.Connection{
			Id:               connId,
			Namespace:        "root",
			State:            database.ConnectionStateCreated,
			ConnectorId:      cv.GetId(),
			ConnectorVersion: cv.GetVersion(),
		},
		s:      s,
		cv:     cv,
		logger: aplog.NewNoopLogger(),
	}
}

func TestConnectionGetSetupStep(t *testing.T) {
	t.Run("returns nil when no setup step set", func(t *testing.T) {
		conn := newTestConnection(cschema.Connector{})
		assert.Nil(t, conn.GetSetupStep())
	})

	t.Run("returns the setup step value", func(t *testing.T) {
		conn := newTestConnection(cschema.Connector{})
		step := "preconnect:0"
		conn.SetupStep = &step
		result := conn.GetSetupStep()
		require.NotNil(t, result)
		assert.Equal(t, "preconnect:0", *result)
	})
}

func TestConnectionSetSetupStep(t *testing.T) {
	t.Run("sets setup step and updates local state", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		s, db, _, _, _, _ := FullMockService(t, ctrl)
		conn := newTestConnectionWithService(s)

		step := "configure:1"
		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, &step).Return(nil)

		err := conn.SetSetupStep(context.Background(), &step)
		require.NoError(t, err)
		require.NotNil(t, conn.SetupStep)
		assert.Equal(t, "configure:1", *conn.SetupStep)
	})

	t.Run("clears setup step when nil", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		s, db, _, _, _, _ := FullMockService(t, ctrl)
		conn := newTestConnectionWithService(s)
		existing := "preconnect:0"
		conn.SetupStep = &existing

		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, (*string)(nil)).Return(nil)

		err := conn.SetSetupStep(context.Background(), nil)
		require.NoError(t, err)
		assert.Nil(t, conn.SetupStep)
	})

	t.Run("returns error from database", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		s, db, _, _, _, _ := FullMockService(t, ctrl)
		conn := newTestConnectionWithService(s)

		step := "preconnect:0"
		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, &step).Return(database.ErrNotFound)

		err := conn.SetSetupStep(context.Background(), &step)
		assert.ErrorIs(t, err, database.ErrNotFound)
		// Local state should not be updated on error
		assert.Nil(t, conn.SetupStep)
	})
}

func TestConnectionGetConfiguration(t *testing.T) {
	t.Run("returns nil when no configuration set", func(t *testing.T) {
		conn := newTestConnection(cschema.Connector{})
		result, err := conn.GetConfiguration(context.Background())
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("returns nil when encrypted configuration is zero value", func(t *testing.T) {
		conn := newTestConnection(cschema.Connector{})
		conn.EncryptedConfiguration = &encfield.EncryptedField{}
		result, err := conn.GetConfiguration(context.Background())
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("decrypts and returns configuration", func(t *testing.T) {
		e := encrypt.NewFakeEncryptService(false)
		s := &service{encrypt: e, logger: aplog.NewNoopLogger()}
		conn := newTestConnectionWithService(s)

		// Simulate stored encrypted configuration
		ef, err := e.EncryptStringForNamespace(context.Background(), "root", `{"tenant":"acme","base_url":"https://acme.example.com"}`)
		require.NoError(t, err)
		conn.EncryptedConfiguration = &ef

		result, err := conn.GetConfiguration(context.Background())
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "acme", result["tenant"])
		assert.Equal(t, "https://acme.example.com", result["base_url"])
	})

	t.Run("returns error on decrypt failure", func(t *testing.T) {
		e := encrypt.NewFakeEncryptService(false)
		s := &service{encrypt: e, logger: aplog.NewNoopLogger()}
		conn := newTestConnectionWithService(s)

		// Set an encrypted field with a non-fake key ID to trigger decrypt error
		conn.EncryptedConfiguration = &encfield.EncryptedField{ID: "ekv_wrong", Data: "some-data"}

		_, err := conn.GetConfiguration(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decrypt")
	})

	t.Run("returns error on invalid JSON", func(t *testing.T) {
		e := encrypt.NewFakeEncryptService(false)
		s := &service{encrypt: e, logger: aplog.NewNoopLogger()}
		conn := newTestConnectionWithService(s)

		ef, err := e.EncryptStringForNamespace(context.Background(), "root", `not-valid-json`)
		require.NoError(t, err)
		conn.EncryptedConfiguration = &ef

		_, err = conn.GetConfiguration(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to unmarshal")
	})
}

func TestConnectionSetConfiguration(t *testing.T) {
	t.Run("encrypts and stores configuration", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		e := encrypt.NewFakeEncryptService(false)
		s, db, _, _, _, _ := FullMockService(t, ctrl)
		s.encrypt = e
		conn := newTestConnectionWithService(s)

		data := map[string]any{
			"tenant":   "acme",
			"base_url": "https://acme.example.com",
		}

		db.EXPECT().SetConnectionEncryptedConfiguration(gomock.Any(), conn.Id, gomock.Any()).
			DoAndReturn(func(ctx context.Context, id apid.ID, ef *encfield.EncryptedField) error {
				require.NotNil(t, ef)
				assert.Equal(t, apid.ID("ekv_fake"), ef.ID)
				assert.NotEmpty(t, ef.Data)
				return nil
			})

		err := conn.SetConfiguration(context.Background(), data)
		require.NoError(t, err)
		require.NotNil(t, conn.EncryptedConfiguration)
		assert.Equal(t, apid.ID("ekv_fake"), conn.EncryptedConfiguration.ID)
	})

	t.Run("round trip set then get", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		e := encrypt.NewFakeEncryptService(false)
		s, db, _, _, _, _ := FullMockService(t, ctrl)
		s.encrypt = e
		conn := newTestConnectionWithService(s)

		data := map[string]any{
			"tenant":     "acme",
			"workspace":  "main",
			"num_value":  float64(42),
			"bool_value": true,
		}

		db.EXPECT().SetConnectionEncryptedConfiguration(gomock.Any(), conn.Id, gomock.Any()).Return(nil)

		err := conn.SetConfiguration(context.Background(), data)
		require.NoError(t, err)

		// Now get it back
		result, err := conn.GetConfiguration(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "acme", result["tenant"])
		assert.Equal(t, "main", result["workspace"])
		assert.Equal(t, float64(42), result["num_value"])
		assert.Equal(t, true, result["bool_value"])
	})

	t.Run("returns error from database", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		e := encrypt.NewFakeEncryptService(false)
		s, db, _, _, _, _ := FullMockService(t, ctrl)
		s.encrypt = e
		conn := newTestConnectionWithService(s)

		db.EXPECT().SetConnectionEncryptedConfiguration(gomock.Any(), conn.Id, gomock.Any()).Return(database.ErrNotFound)

		err := conn.SetConfiguration(context.Background(), map[string]any{"key": "value"})
		assert.ErrorIs(t, err, database.ErrNotFound)
		// Local state should not be updated on error
		assert.Nil(t, conn.EncryptedConfiguration)
	})

	t.Run("overwrites existing configuration", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		e := encrypt.NewFakeEncryptService(false)
		s, db, _, _, _, _ := FullMockService(t, ctrl)
		s.encrypt = e
		conn := newTestConnectionWithService(s)

		db.EXPECT().SetConnectionEncryptedConfiguration(gomock.Any(), conn.Id, gomock.Any()).Return(nil).Times(2)

		// Set initial config
		err := conn.SetConfiguration(context.Background(), map[string]any{"tenant": "acme"})
		require.NoError(t, err)

		// Overwrite with new config
		err = conn.SetConfiguration(context.Background(), map[string]any{"tenant": "newcorp", "extra": "field"})
		require.NoError(t, err)

		result, err := conn.GetConfiguration(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "newcorp", result["tenant"])
		assert.Equal(t, "field", result["extra"])
	})
}
