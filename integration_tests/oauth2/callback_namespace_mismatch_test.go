//go:build integration

package oauth2

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCallbackRejection_NamespaceMismatchActor exercises the
// `namespace_mismatch_actor` defense-in-depth check at
// internal/auth_methods/oauth2/state.go:203-213. The check fires when the
// caller's actor id matches the state's actor id but the namespaces differ
// — a configuration that the natural _initiate flow can't produce because
// actor ids are allocated per (namespace, external_id). The only realistic
// way to land in this state is via a corrupted state envelope (e.g., a
// secondary AuthProxy instance writing into the same Redis with a colliding
// actor id), so the test injects a synthetic state envelope directly.
//
// The check guards against the scenario where two AuthProxy instances share
// Redis and happen to allocate the same actor id in different namespaces.
// Without this check, the actor-id check would pass and the namespace
// boundary would silently fail.
func TestCallbackRejection_NamespaceMismatchActor(t *testing.T) {
	provider := helpers.NewOAuth2TestProvider(t)

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	tenantA := "root.tenant-a-" + suffix
	lyingNamespace := "root.tenant-b-" + suffix
	aliceExternalID := "alice-" + suffix
	clientKey := "ns-mismatch-actor-client-" + suffix
	clientSecret := "ns-mismatch-actor-secret-" + suffix

	connectorID := apid.New(apid.PrefixConnectorVersion)
	connector := helpers.NewOAuth2Connector(connectorID, "ns-mismatch-actor-test", provider, helpers.OAuth2ConnectorOptions{
		ClientID:     clientKey,
		ClientSecret: clientSecret,
		Scopes:       []string{"read"},
	})

	logCapture := helpers.NewLogCapture()
	env := helpers.Setup(t, helpers.SetupOptions{
		Service:       helpers.ServiceTypeAPI,
		IncludePublic: true,
		Connectors:    []sconfig.Connector{connector},
		LogCapture:    logCapture,
	})
	defer env.Cleanup()

	ctx := context.Background()
	_, err := env.Core.CreateNamespace(ctx, tenantA, nil)
	require.NoError(t, err)

	provider.CreateClient(helpers.CreateClientRequest{
		Key:                     clientKey,
		Secret:                  clientSecret,
		RedirectURI:             env.PublicOAuthCallbackURL(),
		TokenEndpointAuthMethod: helpers.TokenEndpointAuthPost,
		Scope:                   "read",
	})

	// Alice initiates a real connection in tenant-a so we have a valid
	// connection row to point the synthetic state at, and so alice's
	// actor row is materialized with an apid we can reference.
	connID, _ := env.InitiateOAuth2Connection(t, connectorID, "https://example.com/return", helpers.WithActor(aliceExternalID, tenantA))
	alice, err := env.Db.GetActorByExternalId(ctx, tenantA, aliceExternalID)
	require.NoError(t, err)
	require.NotNil(t, alice)
	conn := env.GetConnection(t, connID)

	// Synthesize a fresh state envelope that lies about the namespace
	// while keeping the actor id correct. The natural state for this
	// connection (already in Redis at the real state_id from initiate)
	// is left alone; the test delivers the forged envelope at a fresh
	// state id so the assertion targets exactly the synthetic value.
	forgedStateID := apid.New(apid.PrefixOauth2State)
	env.WriteOAuth2StateForTest(t, helpers.OAuth2StateForTest{
		Id:               forgedStateID,
		Namespace:        lyingNamespace,
		ActorId:          alice.Id,
		ConnectorId:      conn.ConnectorId,
		ConnectorVersion: conn.ConnectorVersion,
		ConnectionId:     conn.Id,
		ReturnToUrl:      "https://example.com/return",
		ExpiresAt:        time.Now().Add(5 * time.Minute),
	}, 5*time.Minute)

	// Deliver the callback as alice in tenant-a. The actor-id check passes
	// (alice signed her own JWT), but s.Namespace ("tenant-b") doesn't
	// match the caller's namespace ("tenant-a") — fires
	// `namespace_mismatch_actor` before the connection lookup runs.
	loc := env.DeliverOAuth2Callback(t,
		env.ForgeOAuth2CallbackURL(forgedStateID.String(), "fake-code"),
		helpers.WithActor(aliceExternalID, tenantA),
	)
	errorPageURL := env.Cfg.GetRoot().ErrorPages.InternalError
	require.NotEmpty(t, errorPageURL, "test config must set error_pages.internal_error")
	assert.Equal(t, errorPageURL, loc, "callback should redirect to error page on rejection")

	events := logCapture.RecordsWithMessage(t, rejectionEventMessage)
	require.Lenf(t, events, 1, "expected exactly one rejection event; got %d (%v)", len(events), events)
	assert.Equal(t, "namespace_mismatch_actor", events[0]["category"])
	assert.Equal(t, forgedStateID.String(), events[0]["state_id"])

	// Real connection from initiate should still have no token row — the
	// rejected callback never reached the token exchange.
	require.Nil(t, env.GetOAuth2Token(t, connID))
	connAfter := env.GetConnection(t, connID)
	assert.Equal(t, database.ConnectionStateCreated, connAfter.State)

	tokenReqs := provider.Requests(helpers.RequestsFilter{
		Endpoint: helpers.EndpointToken,
		ClientID: clientKey,
	})
	assert.Empty(t, tokenReqs, "provider must not have observed a /token call when rejected")
}

// TestCallbackRejection_NamespaceMismatchConnection exercises the
// `namespace_mismatch_connection` defense-in-depth check at
// internal/auth_methods/oauth2/state.go:232-242. The check fires when
// state.Namespace and the *connection's* namespace disagree — even
// after the caller-vs-state namespace check passes. A natural _initiate
// always produces matching namespaces, so this also requires synthetic
// state injection.
//
// Scenario: bob owns a connection in tenant-b. An attacker forges a state
// envelope that points the connection_id at bob's connection but claims
// the state belongs to alice in tenant-a, and delivers the callback as
// alice. The actor-id and actor-namespace checks pass (alice signs her
// own JWT into tenant-a, state claims tenant-a), but the connection
// itself lives in tenant-b — the third check rejects.
func TestCallbackRejection_NamespaceMismatchConnection(t *testing.T) {
	provider := helpers.NewOAuth2TestProvider(t)

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	tenantA := "root.tenant-a-" + suffix
	tenantB := "root.tenant-b-" + suffix
	aliceExternalID := "alice-" + suffix
	bobExternalID := "bob-" + suffix
	clientKey := "ns-mismatch-conn-client-" + suffix
	clientSecret := "ns-mismatch-conn-secret-" + suffix

	connectorID := apid.New(apid.PrefixConnectorVersion)
	connector := helpers.NewOAuth2Connector(connectorID, "ns-mismatch-conn-test", provider, helpers.OAuth2ConnectorOptions{
		ClientID:     clientKey,
		ClientSecret: clientSecret,
		Scopes:       []string{"read"},
	})

	logCapture := helpers.NewLogCapture()
	env := helpers.Setup(t, helpers.SetupOptions{
		Service:       helpers.ServiceTypeAPI,
		IncludePublic: true,
		Connectors:    []sconfig.Connector{connector},
		LogCapture:    logCapture,
	})
	defer env.Cleanup()

	ctx := context.Background()
	_, err := env.Core.CreateNamespace(ctx, tenantA, nil)
	require.NoError(t, err)
	_, err = env.Core.CreateNamespace(ctx, tenantB, nil)
	require.NoError(t, err)

	provider.CreateClient(helpers.CreateClientRequest{
		Key:                     clientKey,
		Secret:                  clientSecret,
		RedirectURI:             env.PublicOAuthCallbackURL(),
		TokenEndpointAuthMethod: helpers.TokenEndpointAuthPost,
		Scope:                   "read",
	})

	// Bob owns a connection in tenant-b. We'll point the synthetic state
	// at this connection to make the connection-namespace check fail.
	bobConnID, _ := env.InitiateOAuth2Connection(t, connectorID, "https://example.com/return", helpers.WithActor(bobExternalID, tenantB))
	bobConn := env.GetConnection(t, bobConnID)
	require.Equal(t, tenantB, bobConn.Namespace, "bob's connection should live in tenant-b")

	// Materialize alice's actor by initiating a throwaway connection in
	// tenant-a (we don't reference this connection in the synthetic state;
	// we only need alice's actor_id so the actor-id check passes).
	_, _ = env.InitiateOAuth2Connection(t, connectorID, "https://example.com/return", helpers.WithActor(aliceExternalID, tenantA))
	alice, err := env.Db.GetActorByExternalId(ctx, tenantA, aliceExternalID)
	require.NoError(t, err)

	// Synthetic state: ActorId+Namespace agree with alice's caller
	// identity, but ConnectionId points at bob's connection in tenant-b.
	// The first two checks pass; the third (state.Namespace vs
	// connection.Namespace) rejects.
	forgedStateID := apid.New(apid.PrefixOauth2State)
	env.WriteOAuth2StateForTest(t, helpers.OAuth2StateForTest{
		Id:               forgedStateID,
		Namespace:        tenantA,
		ActorId:          alice.Id,
		ConnectorId:      bobConn.ConnectorId,
		ConnectorVersion: bobConn.ConnectorVersion,
		ConnectionId:     bobConn.Id,
		ReturnToUrl:      "https://example.com/return",
		ExpiresAt:        time.Now().Add(5 * time.Minute),
	}, 5*time.Minute)

	loc := env.DeliverOAuth2Callback(t,
		env.ForgeOAuth2CallbackURL(forgedStateID.String(), "fake-code"),
		helpers.WithActor(aliceExternalID, tenantA),
	)
	errorPageURL := env.Cfg.GetRoot().ErrorPages.InternalError
	require.NotEmpty(t, errorPageURL, "test config must set error_pages.internal_error")
	assert.Equal(t, errorPageURL, loc, "callback should redirect to error page on rejection")

	events := logCapture.RecordsWithMessage(t, rejectionEventMessage)
	require.Lenf(t, events, 1, "expected exactly one rejection event; got %d (%v)", len(events), events)
	assert.Equal(t, "namespace_mismatch_connection", events[0]["category"])
	assert.Equal(t, forgedStateID.String(), events[0]["state_id"])

	// Bob's connection in tenant-b must not be modified — the forged
	// callback claimed alice's identity but bob's connection was not
	// actually touched (no token attached, state still `created`).
	require.Nil(t, env.GetOAuth2Token(t, bobConnID),
		"bob's connection must not have a token attached after a rejected forgery")
	bobConnAfter := env.GetConnection(t, bobConnID)
	assert.Equal(t, database.ConnectionStateCreated, bobConnAfter.State,
		"bob's connection state should remain `created`")
	assert.Nil(t, bobConnAfter.SetupStep)
	assert.Nil(t, bobConnAfter.SetupError)

	tokenReqs := provider.Requests(helpers.RequestsFilter{
		Endpoint: helpers.EndpointToken,
		ClientID: clientKey,
	})
	assert.Empty(t, tokenReqs, "provider must not have observed a /token call when rejected")
}
