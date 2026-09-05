package core

import (
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	connectionschema "github.com/rmorlok/authproxy/internal/schema/resources/connection"
	connectorschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
)

func TestConnectionGetResourceBuildsCanonicalEnvelope(t *testing.T) {
	now := time.Now().UTC()
	actorID := apid.New(apid.PrefixActor)
	setupError := "authorization failed"
	connectionID := apid.New(apid.PrefixConnection)
	connectorID := apid.New(apid.PrefixConnector)
	connector := &Connector{ConnectorWithDefinition: database.ConnectorWithDefinition{
		Id:        connectorID,
		Name:      "salesforce",
		Namespace: "root.acme",
		Version:   4,
	}}
	encryptService := encrypt.NewFakeEncryptService(false)
	encryptedConfiguration, err := encryptService.EncryptStringForNamespace(t.Context(), "root.acme.team", `{"tenant":"acme"}`)
	require.NoError(t, err)
	service := &service{encrypt: encryptService, logger: aplog.NewNoopLogger()}
	wrapped := wrapConnection(&database.Connection{
		Id:                     connectionID,
		Name:                   "production",
		Namespace:              "root.acme.team",
		State:                  database.ConnectionStateSetup,
		HealthState:            database.ConnectionHealthStateUnhealthy,
		ConnectorId:            connectorID,
		ConnectorVersion:       4,
		ActorId:                &actorID,
		Labels:                 database.Labels{"team": "platform"},
		Annotations:            database.Annotations{"owner": "integrations"},
		SetupStep:              &connectorschema.SetupStepVerifyFailed,
		SetupError:             &setupError,
		EncryptedConfiguration: &encryptedConfiguration,
		CreatedAt:              now,
		UpdatedAt:              now,
	}, connector, service)

	resource, err := wrapped.GetResource(t.Context())
	require.NoError(t, err)
	require.Equal(t, meta.APIVersionV1Alpha1, resource.APIVersion)
	require.Equal(t, connectionschema.ConnectionKind, resource.Kind)
	require.Equal(t, connectionID.String(), resource.Metadata.ID)
	require.Equal(t, "platform", resource.Metadata.Labels["team"])
	require.Equal(t, connectorID.String(), resource.Spec.ConnectorRef.ID)
	require.Equal(t, uint64(4), resource.Spec.ConnectorRef.Generation)
	require.Equal(t, actorID.String(), resource.Spec.ActorRef.ID)
	require.Equal(t, "****", resource.Spec.Configuration["tenant"])
	require.Equal(t, connectionschema.ConnectionStateSetup, resource.Status.Lifecycle.State)
	require.Equal(t, connectionschema.ConnectionHealthStateUnhealthy, resource.Status.Health.State)
	require.Equal(t, connectorschema.SetupStepVerifyFailed.String(), resource.Status.Setup.StepID)
	require.Equal(t, setupError, *resource.Status.Setup.Error)
	require.True(t, resource.Status.ConfigurationConfigured)
	require.NoError(t, resource.ValidateFor(meta.ValidationModeResponse, nil))

	resource.Metadata.Labels["team"] = "changed"
	resource.Spec.ActorRef.ID = apid.New(apid.PrefixActor).String()
	require.Equal(t, "platform", wrapped.Labels["team"])
	require.Equal(t, actorID, *wrapped.ActorId)
	require.Equal(t, actorID, *wrapped.GetActorId())
}

func TestConnectionGetResourceOmitsOptionalActorAndSetup(t *testing.T) {
	now := time.Now().UTC()
	connector := &Connector{ConnectorWithDefinition: database.ConnectorWithDefinition{
		Id:        apid.New(apid.PrefixConnector),
		Name:      "example",
		Namespace: "root",
		Version:   1,
	}}
	wrapped := wrapConnection(&database.Connection{
		Id:               apid.New(apid.PrefixConnection),
		Name:             "example",
		Namespace:        "root",
		State:            database.ConnectionStateConfigured,
		ConnectorId:      connector.Id,
		ConnectorVersion: connector.Version,
		CreatedAt:        now,
		UpdatedAt:        now,
	}, connector, &service{logger: aplog.NewNoopLogger()})

	resource, err := wrapped.GetResource(t.Context())
	require.NoError(t, err)
	require.Nil(t, resource.Spec.ActorRef)
	require.Nil(t, resource.Status.Setup)
	require.False(t, resource.Status.ConfigurationConfigured)
	require.Equal(t, connectionschema.ConnectionHealthStateHealthy, resource.Status.Health.State)
	require.NoError(t, resource.ValidateFor(meta.ValidationModeResponse, nil))
}
