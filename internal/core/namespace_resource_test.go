package core

import (
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	nschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/stretchr/testify/require"
)

func TestNamespaceResourceFromDatabase(t *testing.T) {
	createdAt := time.Date(2026, 8, 30, 10, 0, 0, 0, time.FixedZone("offset", -5*60*60))
	updatedAt := createdAt.Add(time.Minute)
	keyID := apid.New(apid.PrefixKey)
	dbNamespace := database.Namespace{
		Path:        "root.acme.billing",
		State:       database.NamespaceStateActive,
		KeyId:       &keyID,
		Labels:      database.Labels{"team": "platform"},
		Annotations: database.Annotations{"example.com/owner": "integrations"},
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}

	resource := namespaceResourceFromDatabase(dbNamespace)
	require.Equal(t, meta.APIVersionV1Alpha1, resource.APIVersion)
	require.Equal(t, nschema.NamespaceKind, resource.Kind)
	require.Equal(t, "root.acme.billing", resource.Metadata.ID)
	require.Equal(t, "billing", string(resource.Metadata.Name))
	require.Equal(t, "root.acme", resource.Metadata.Namespace)
	require.Equal(t, nschema.NamespaceStateActive, resource.Status.State)
	require.Equal(t, keyID.String(), resource.Spec.EncryptionKeyRef.ID)
	require.Equal(t, time.UTC, resource.Metadata.CreatedAt.Location())
	require.Equal(t, createdAt.UTC(), *resource.Metadata.CreatedAt)

	resource.Metadata.Labels["team"] = "changed"
	resource.Metadata.Annotations["example.com/owner"] = "changed"
	require.Equal(t, "platform", dbNamespace.Labels["team"])
	require.Equal(t, "integrations", dbNamespace.Annotations["example.com/owner"])
}

func TestDatabaseNamespaceFromResource(t *testing.T) {
	resource, err := nschema.NewNamespaceForPath("root.acme")
	require.NoError(t, err)
	resource.Metadata.Labels = map[string]string{"team": "platform"}
	resource.Metadata.Annotations = map[string]string{"example.com/owner": "integrations"}
	keyID := apid.New(apid.PrefixKey)
	resource.Spec.EncryptionKeyRef = &meta.ObjectReference{
		APIVersion: meta.APIVersionV1Alpha1,
		Kind:       "Key",
		ID:         keyID.String(),
	}

	dbNamespace, err := databaseNamespaceFromResource(resource)
	require.NoError(t, err)
	require.Equal(t, "root.acme", dbNamespace.Path)
	require.Equal(t, database.NamespaceStateActive, dbNamespace.State)
	require.Equal(t, keyID, *dbNamespace.KeyId)
	require.Equal(t, database.Labels{"team": "platform"}, dbNamespace.Labels)
	require.Equal(t, database.Annotations{"example.com/owner": "integrations"}, dbNamespace.Annotations)

	dbNamespace.Labels["team"] = "changed"
	dbNamespace.Annotations["example.com/owner"] = "changed"
	require.Equal(t, "platform", resource.Metadata.Labels["team"])
	require.Equal(t, "integrations", resource.Metadata.Annotations["example.com/owner"])

	root, err := nschema.NewNamespaceForPath(nschema.Root)
	require.NoError(t, err)
	dbRoot, err := databaseNamespaceFromResource(root)
	require.NoError(t, err)
	require.Equal(t, nschema.Root, dbRoot.Path)

	namedKey, err := nschema.NewNamespaceForPath("root.named-key")
	require.NoError(t, err)
	namedKey.Spec.EncryptionKeyRef = &meta.ObjectReference{
		APIVersion: meta.APIVersionV1Alpha1,
		Kind:       nschema.EncryptionKeyKind,
		Namespace:  "root",
		Name:       "key_global",
	}
	dbNamedKey, err := databaseNamespaceFromResource(namedKey)
	require.NoError(t, err)
	require.Nil(t, dbNamedKey.KeyId)
}
