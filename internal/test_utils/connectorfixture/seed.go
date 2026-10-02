// Package connectorfixture seeds fixed connector identities for tests. Production
// provisioning uses the resource API via ap apply.
package connectorfixture

import (
	"context"
	"errors"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/stretchr/testify/require"
)

func Seed(t testing.TB, db database.DB, e encrypt.E, resources ...cschema.Connector) {
	t.Helper()
	ctx := context.Background()
	for _, resource := range resources {
		ns := resource.GetNamespace()
		for _, path := range namespace.SplitPathsToPrefixes([]string{ns}) {
			_, err := db.GetNamespace(ctx, path)
			if errors.Is(err, database.ErrNotFound) {
				err = db.CreateNamespace(ctx, &database.Namespace{Path: path, State: database.NamespaceStateActive})
			}
			require.NoError(t, err)
		}
		id := resource.GetId()
		if id == apid.Nil {
			id = apid.New(apid.PrefixConnector)
		}
		generation := resource.Metadata.Generation
		if generation == 0 {
			generation = 1
		}
		state := database.ConnectorGenerationState(resource.Spec.Release.DesiredState)
		if state == "" {
			state = database.ConnectorGenerationStatePrimary
		}
		definition, err := meta.CanonicalSpecJSON(&resource.Spec.Definition)
		require.NoError(t, err)
		encrypted, err := e.EncryptStringForEntity(ctx, &resource, string(definition))
		require.NoError(t, err)
		require.NoError(t, db.UpsertConnectorGeneration(ctx, &database.ConnectorWithDefinition{
			Id: id, Generation: generation, Namespace: ns, Name: resource.Metadata.Name,
			Labels: resource.Metadata.Labels, Annotations: resource.Metadata.Annotations,
			State: state, EncryptedDefinition: encrypted,
		}))
	}
}
