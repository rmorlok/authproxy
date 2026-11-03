package core

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	authcore "github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/httpf"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/require"
)

// toolPreparationFixture starts after authorization with a configured connection;
// strict dependency mocks reject rereads, credential access, and HTTP setup.
func toolPreparationFixture(t *testing.T) (*toolAuthorizationFixture, *authorizedToolInvocation) {
	t.Helper()
	f := newToolAuthorizationFixture(t, []string{"tool:read"})
	f.snapshot.Connection.State = database.ConnectionStateConfigured
	return f, &authorizedToolInvocation{
		snapshot: f.snapshot, definition: f.definition,
		actor: &authcore.Actor{Id: apid.New(apid.PrefixActor), Namespace: "root"},
	}
}

// expectPreparationDecryption permits only the captured connector definition and
// configuration to be read, in that order and with the caller's context.
func expectPreparationDecryption(f *toolAuthorizationFixture, ctx context.Context, configuration string) {
	gomock.InOrder(
		f.encrypt.EXPECT().DecryptString(ctx, f.snapshot.Connector.EncryptedDefinition).
			Return(`{"auth":{"type":"no-auth"},"description":"captured generation"}`, nil),
		f.encrypt.EXPECT().DecryptString(ctx, *f.snapshot.Connection.EncryptedConfiguration).
			Return(configuration, nil),
	)
}

// TestPrepareToolInvocationCapturedContext carries one authorized database capture
// into both template input and a lazily configured connection HTTP client.
func TestPrepareToolInvocationCapturedContext(t *testing.T) {
	f := newToolAuthorizationFixture(t, []string{"tool:read"})
	f.snapshot.Connection.State = database.ConnectionStateConfigured
	f.snapshot.Connection.Labels = database.Labels{"owner": "captured-owner"}
	f.snapshot.Connection.Annotations = database.Annotations{"note": "captured-note"}
	ctx := context.Background()
	auth := toolInvocationAuth(toolInvocationGrant("tool:read"), nil)
	auth.GetActor().Labels = map[string]string{"team": "captured-team"}
	f.expectCapturedDefinition()
	expectPreparationDecryption(f, ctx, `{"tenant":"captured-tenant","nested":{"values":["first"]}}`)

	// Authorization and preparation share the captured rows; preparation adds no database reads.
	authorized, err := f.service.authorizeToolInvocation(ctx, auth, f.snapshot.Revision.ToolId, nil)
	require.NoError(t, err)
	// Later changes to the request actor cannot alter the lazy client's attribution.
	auth.GetActor().Labels["team"] = "changed after authorization"
	got, err := f.service.prepareToolInvocation(ctx, authorized)
	require.NoError(t, err)
	require.Nil(t, got.snapshot.Params)
	require.Equal(t, f.snapshot.Connection.Id.String(), got.snapshot.ConnectionID)
	require.Equal(t, f.snapshot.Connection.Namespace, got.snapshot.ConnectionNamespace)
	require.Equal(t, f.snapshot.Revision.ToolId.String(), got.snapshot.ToolID)
	require.Equal(t, f.snapshot.Revision.Revision, got.snapshot.Revision)
	require.Equal(t, f.snapshot.ToolSetId.String(), got.snapshot.ToolSetID)
	require.Equal(t, f.snapshot.Revision.SourceGeneration, got.snapshot.ToolSetGeneration)
	require.Nil(t, got.snapshot.CatalogRevision)
	require.Equal(t, "captured-owner", got.snapshot.Labels["owner"])
	require.Equal(t, "captured-note", got.snapshot.Annotations["note"])
	require.Equal(t, "captured-tenant", got.snapshot.Cfg["tenant"])

	// The client already holds the selected connector generation and decoded configuration.
	client, ok := got.client.(*connectionHTTPClient)
	require.True(t, ok)
	require.Equal(t, f.snapshot.Connection.Id, client.connection.Id)
	require.Equal(t, uint64(4), client.connection.connector.Generation)
	require.Equal(t, "captured generation", client.connection.connector.GetDefinition().Description)
	require.Equal(t, httpf.RequestTypeProxy, client.options.RequestType)
	require.Equal(t, authorized.actor.Id, client.options.Actor.Actor().GetId())
	require.Equal(t, "captured-team", client.options.Actor.Actor().GetLabels()["team"])
	require.Nil(t, client.client)
	require.Nil(t, client.connection.proxyImpl)

	// Render data can be changed without modifying the client's cached configuration or metadata.
	got.snapshot.Cfg["tenant"] = "changed"
	got.snapshot.Cfg["nested"].(map[string]any)["values"].([]any)[0] = "changed"
	got.snapshot.Labels["owner"] = "changed"
	got.snapshot.Annotations["note"] = "changed"
	*got.snapshot.ToolSetGeneration = 99
	config, err := client.connection.GetConfiguration(ctx)
	require.NoError(t, err)
	require.Equal(t, "captured-tenant", config["tenant"])
	require.Equal(t, "first", config["nested"].(map[string]any)["values"].([]any)[0])
	require.Equal(t, "captured-owner", client.connection.Labels["owner"])
	require.Equal(t, "captured-note", client.connection.Annotations["note"])
	require.Equal(t, uint64(3), *f.snapshot.Revision.SourceGeneration)
}

// TestPrepareToolInvocationUnavailable rejects lifecycle states before decrypting
// private configuration or resolving any connector capability.
func TestPrepareToolInvocationUnavailable(t *testing.T) {
	for _, state := range []database.ConnectionState{
		database.ConnectionStateSetup, database.ConnectionStateDisabled,
		database.ConnectionStateDisconnecting, database.ConnectionStateDisconnected, "",
	} {
		t.Run(string(state), func(t *testing.T) {
			// Strict mocks have no expectations: a nonconfigured connection cannot progress.
			f, authorized := toolPreparationFixture(t)
			f.snapshot.Connection.State = state
			got, err := f.service.prepareToolInvocation(context.Background(), authorized)
			require.ErrorIs(t, err, errToolConnectionUnavailable)
			require.Nil(t, got)
		})
	}
}

// TestPrepareToolInvocationConfiguredHealth permits a configured connection whose
// probes are unhealthy or whose reconfiguration flow is currently active.
func TestPrepareToolInvocationConfiguredHealth(t *testing.T) {
	f, authorized := toolPreparationFixture(t)
	f.snapshot.Connection.HealthState = database.ConnectionHealthStateUnhealthy
	step := cschema.MustNewSetupStep("tenant")
	f.snapshot.Connection.SetupStep = &step
	expectPreparationDecryption(f, context.Background(), `{}`)

	// Setup progress and health are separate from the configured lifecycle state.
	got, err := f.service.prepareToolInvocation(context.Background(), authorized)
	require.NoError(t, err)
	require.NotNil(t, got)
}

// TestPrepareToolInvocationEmptyConfiguration omits absent encrypted configuration
// and ToolSet provenance rather than inventing values or decrypting empty fields.
func TestPrepareToolInvocationEmptyConfiguration(t *testing.T) {
	for _, configuration := range []*encfield.EncryptedField{nil, {}} {
		t.Run("empty configuration", func(t *testing.T) {
			f, authorized := toolPreparationFixture(t)
			f.snapshot.Connection.EncryptedConfiguration = configuration
			f.snapshot.ToolSetId = apid.Nil
			f.snapshot.Revision.SourceGeneration = nil
			f.encrypt.EXPECT().DecryptString(gomock.Any(), f.snapshot.Connector.EncryptedDefinition).
				Return(`{"auth":{"type":"no-auth"}}`, nil)

			// A standalone Tool with no configuration still has a usable lazy client.
			got, err := f.service.prepareToolInvocation(context.Background(), authorized)
			require.NoError(t, err)
			require.Empty(t, got.snapshot.Cfg)
			require.Empty(t, got.snapshot.ToolSetID)
			require.Nil(t, got.snapshot.ToolSetGeneration)
			require.NotNil(t, got.client)
		})
	}
}

// TestPrepareToolInvocationUnsupportedAuth stops before configuration decryption
// when the captured connector has no registered proxy authentication method.
func TestPrepareToolInvocationUnsupportedAuth(t *testing.T) {
	f, authorized := toolPreparationFixture(t)
	f.encrypt.EXPECT().DecryptString(gomock.Any(), f.snapshot.Connector.EncryptedDefinition).
		Return(`{"description":"no authentication capability"}`, nil)

	// Missing auth is a capability error, not a reason to inspect private configuration.
	got, err := f.service.prepareToolInvocation(context.Background(), authorized)
	require.ErrorIs(t, err, ErrProxyNotImplemented)
	require.Nil(t, got)
}

// TestPrepareToolInvocationPreconditions rejects incomplete internal captures and
// an already canceled request before invoking any dependency.
func TestPrepareToolInvocationPreconditions(t *testing.T) {
	for _, name := range []string{"nil authorized", "nil snapshot", "nil actor", "missing actor ID", "canceled"} {
		t.Run(name, func(t *testing.T) {
			f, authorized := toolPreparationFixture(t)
			ctx, want := context.Background(), ErrInvalidArgument
			switch name {
			case "nil authorized":
				authorized = nil
			case "nil snapshot":
				authorized.snapshot = nil
			case "nil actor":
				authorized.actor = nil
			case "missing actor ID":
				authorized.actor.Id = apid.Nil
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
				want = context.Canceled
			}

			// Strict mocks ensure invalid inputs do not reach decryption or client setup.
			got, err := f.service.prepareToolInvocation(ctx, authorized)
			require.ErrorIs(t, err, want)
			require.Nil(t, got)
		})
	}
}

// TestPrepareToolInvocationDependencyErrors returns no usable context or client
// when either captured encrypted value cannot be decrypted or decoded.
func TestPrepareToolInvocationDependencyErrors(t *testing.T) {
	failure := errors.New("encryption unavailable")
	for _, name := range []string{"connector decrypt", "connector decode", "configuration decrypt", "configuration decode"} {
		t.Run(name, func(t *testing.T) {
			f, authorized := toolPreparationFixture(t)
			connector, config := `{"auth":{"type":"no-auth"}}`, `{}`
			var connectorErr, configErr error
			switch name {
			case "connector decrypt":
				connectorErr = failure
			case "connector decode":
				connector = `{"auth":`
			case "configuration decrypt":
				configErr = failure
			case "configuration decode":
				config = `["not an object"]`
			}
			f.encrypt.EXPECT().DecryptString(gomock.Any(), f.snapshot.Connector.EncryptedDefinition).
				Return(connector, connectorErr)
			if name == "configuration decrypt" || name == "configuration decode" {
				f.encrypt.EXPECT().DecryptString(gomock.Any(), *f.snapshot.Connection.EncryptedConfiguration).
					Return(config, configErr)
			}

			// A failed connector stops before configuration; every failure discards partial preparation.
			got, err := f.service.prepareToolInvocation(context.Background(), authorized)
			require.Error(t, err)
			require.Nil(t, got)
			if connectorErr != nil || configErr != nil {
				require.ErrorIs(t, err, failure)
			}
		})
	}
}

// TestPrepareToolInvocationCancellation propagates the caller's context and stops
// between dependency operations even when decryption completes without an error.
func TestPrepareToolInvocationCancellation(t *testing.T) {
	for _, stage := range []string{"connector", "configuration"} {
		t.Run(stage, func(t *testing.T) {
			f, authorized := toolPreparationFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f.encrypt.EXPECT().DecryptString(ctx, f.snapshot.Connector.EncryptedDefinition).
				DoAndReturn(func(context.Context, encfield.EncryptedField) (string, error) {
					if stage == "connector" {
						cancel()
					}
					return `{"auth":{"type":"no-auth"}}`, nil
				})
			if stage == "configuration" {
				f.encrypt.EXPECT().DecryptString(ctx, *f.snapshot.Connection.EncryptedConfiguration).
					DoAndReturn(func(context.Context, encfield.EncryptedField) (string, error) {
						cancel()
						return `{}`, nil
					})
			}

			// Cancellation must prevent a prepared invocation from escaping either stage.
			got, err := f.service.prepareToolInvocation(ctx, authorized)
			require.ErrorIs(t, err, context.Canceled)
			require.Nil(t, got)
		})
	}
}
