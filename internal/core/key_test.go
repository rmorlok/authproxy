package core

import (
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encfield"
	keyschema "github.com/rmorlok/authproxy/internal/schema/resources/key"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
)

func TestKeyResourcePersistenceBoundary(t *testing.T) {
	createdAt := time.Date(2026, 9, 1, 8, 0, 0, 0, time.FixedZone("offset", -5*60*60))
	updatedAt := createdAt.Add(time.Minute)
	row := database.Key{
		Id:               apid.MustParse("key_test550e8400abcd"),
		Namespace:        "root.acme",
		Name:             "primary-key",
		Usage:            database.KeyUsageDataEncryption,
		MaterialType:     database.KeyMaterialTypeSymmetric,
		State:            database.KeyStateActive,
		Labels:           database.Labels{"team": "security"},
		Annotations:      database.Annotations{"example.com/purpose": "database"},
		CreatedAt:        createdAt,
		UpdatedAt:        updatedAt,
		EncryptedKeyData: &encfield.EncryptedField{ID: "dek_test", Data: "ciphertext"},
	}

	resource := keyResourceFromDatabase(row)
	require.NoError(t, resource.ValidateFor(meta.ValidationModeResponse, nil))
	require.Equal(t, row.Id.String(), resource.Metadata.ID)
	require.Equal(t, keyschema.KeyUsageDataEncryption, resource.Spec.Usage)
	require.Equal(t, keyschema.KeyMaterialTypeSymmetric, resource.Spec.MaterialType)
	require.Equal(t, keyschema.KeyStateActive, resource.Spec.DesiredState)
	require.Nil(t, resource.Spec.KeyData, "encrypted persistence data must not cross into the public resource")
	require.True(t, resource.Status.KeyDataConfigured)
	require.Equal(t, time.UTC, resource.Metadata.CreatedAt.Location())

	resource.Metadata.Labels["team"] = "changed"
	require.Equal(t, "security", row.Labels["team"])

	resource.Spec.KeyData = &keyschema.KeyData{InnerVal: &keyschema.KeyDataRandomBytes{NumBytes: 32}}
	converted, err := databaseKeyFromResource(resource, row.Id)
	require.NoError(t, err)
	require.Equal(t, row.Id, converted.Id)
	require.Equal(t, database.KeyUsageDataEncryption, converted.Usage)
	require.Equal(t, database.KeyMaterialTypeSymmetric, converted.MaterialType)
	require.Equal(t, database.KeyStateActive, converted.State)
	require.Nil(t, converted.EncryptedKeyData, "encryption remains a separate service responsibility")
}
