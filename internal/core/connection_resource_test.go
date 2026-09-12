package core

import (
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/schema/common"
	connectionschema "github.com/rmorlok/authproxy/internal/schema/resources/connection"
	connectorschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
)

func TestConnectionGetResourceBuildsCanonicalEnvelope(t *testing.T) {
	now := time.Now().UTC()
	setupError := "authorization failed"
	connectionID := apid.New(apid.PrefixConnection)
	connectorID := apid.New(apid.PrefixConnector)
	connector := &Connector{
		ConnectorWithDefinition: database.ConnectorWithDefinition{
			Id:         connectorID,
			Name:       "salesforce",
			Namespace:  "root.acme",
			Generation: 4,
		}, def: &connectorschema.ConnectorDefinition{
			SetupFlow: &connectorschema.SetupFlow{
				Preconnect: &connectorschema.SetupFlowPhase{
					Steps: []connectorschema.SetupFlowStep{
						{
							Id:         "tenant",
							JsonSchema: common.RawJSON(`{"type":"object","required":["tenant"],"properties":{"tenant":{"type":"string"}}}`),
						},
					}},
			}}}

	encryptService := encrypt.NewFakeEncryptService(false)
	encryptedConfiguration, err := encryptService.EncryptStringForNamespace(
		t.Context(),
		"root.acme.team",
		`{"tenant":"acme"}`,
	)
	require.NoError(t, err)

	service := &service{encrypt: encryptService, logger: aplog.NewNoopLogger()}

	wrapped := wrapConnection(
		&database.Connection{
			Id:                     connectionID,
			Name:                   "production",
			Namespace:              "root.acme.team",
			State:                  database.ConnectionStateSetup,
			HealthState:            database.ConnectionHealthStateUnhealthy,
			ConnectorId:            connectorID,
			ConnectorGeneration:    4,
			Labels:                 database.Labels{"team": "platform"},
			Annotations:            database.Annotations{"owner": "integrations"},
			SetupStep:              &connectorschema.SetupStepVerifyFailed,
			SetupError:             &setupError,
			EncryptedConfiguration: &encryptedConfiguration,
			CreatedAt:              now,
			UpdatedAt:              now,
		},
		connector,
		service,
	)

	resource, err := wrapped.GetResource(t.Context())
	require.NoError(t, err)

	require.Equal(t, meta.APIVersionV1Alpha1, resource.APIVersion)
	require.Equal(t, connectionschema.ConnectionKind, resource.Kind)
	require.Equal(t, connectionID.String(), resource.Metadata.ID)
	require.Equal(t, "platform", resource.Metadata.Labels["team"])
	require.Equal(t, connectorID.String(), resource.Spec.ConnectorRef.ID)
	require.Equal(t, uint64(4), resource.Spec.ConnectorRef.Generation)
	require.Equal(t, "acme", resource.Spec.Configuration["tenant"])
	require.JSONEq(t, `{
		"$schema":"https://json-schema.org/draft/2020-12/schema",
		"type":"object",
		"required":["tenant"],
		"properties":{"tenant":{"type":"string"}},
		"additionalProperties":true
	}`, string(resource.Status.Configuration.Schema))
	require.Equal(t, connectionschema.ConnectionStateSetup, resource.Status.Lifecycle.State)
	require.Equal(t, connectionschema.ConnectionHealthStateUnhealthy, resource.Status.Health.State)
	require.Equal(t, connectorschema.SetupStepVerifyFailed.String(), resource.Status.Setup.StepID)
	require.Equal(t, setupError, *resource.Status.Setup.Error)
	require.True(t, resource.Status.Configuration.Configured)
	require.NoError(t, resource.ValidateFor(meta.ValidationModeResponse, nil))

	resource.Metadata.Labels["team"] = "changed"
	require.Equal(t, "platform", wrapped.Labels["team"])
}

func TestConnectionGetResourceOmitsSetupAndDefaultsHealth(t *testing.T) {
	now := time.Now().UTC()
	connector := &Connector{
		ConnectorWithDefinition: database.ConnectorWithDefinition{
			Id:         apid.New(apid.PrefixConnector),
			Name:       "example",
			Namespace:  "root",
			Generation: 1,
		},
		def: &connectorschema.ConnectorDefinition{},
	}
	wrapped := wrapConnection(&database.Connection{
		Id:                  apid.New(apid.PrefixConnection),
		Name:                "example",
		Namespace:           "root",
		State:               database.ConnectionStateConfigured,
		ConnectorId:         connector.Id,
		ConnectorGeneration: connector.Generation,
		CreatedAt:           now,
		UpdatedAt:           now,
	},
		connector,
		&service{logger: aplog.NewNoopLogger()},
	)

	resource, err := wrapped.GetResource(t.Context())
	require.NoError(t, err)

	require.Nil(t, resource.Status.Setup)
	require.True(t, resource.Status.Configuration.Configured)
	require.JSONEq(t, `{
		"$schema":"https://json-schema.org/draft/2020-12/schema",
		"type":"object",
		"properties":{},
		"additionalProperties":true
	}`, string(resource.Status.Configuration.Schema))
	require.Equal(t, connectionschema.ConnectionHealthStateHealthy, resource.Status.Health.State)
	require.NoError(t, resource.ValidateFor(meta.ValidationModeResponse, nil))
}

func TestConnectionGetResourceReportsIncompleteRequiredConfiguration(t *testing.T) {
	now := time.Now().UTC()
	connector := &Connector{
		ConnectorWithDefinition: database.ConnectorWithDefinition{
			Id:         apid.New(apid.PrefixConnector),
			Name:       "example",
			Namespace:  "root",
			Generation: 1,
		},
		def: &connectorschema.ConnectorDefinition{SetupFlow: &connectorschema.SetupFlow{
			Configure: &connectorschema.SetupFlowPhase{Steps: []connectorschema.SetupFlowStep{{
				Id:         "workspace",
				JsonSchema: common.RawJSON(`{"type":"object","required":["workspace"],"properties":{"workspace":{"type":"string"}}}`),
			}}},
		}},
	}

	wrapped := wrapConnection(
		&database.Connection{
			Id:                  apid.New(apid.PrefixConnection),
			Name:                "example",
			Namespace:           "root",
			State:               database.ConnectionStateSetup,
			ConnectorId:         connector.Id,
			ConnectorGeneration: connector.Generation,
			CreatedAt:           now,
			UpdatedAt:           now,
		},
		connector,
		&service{logger: aplog.NewNoopLogger()},
	)

	resource, err := wrapped.GetResource(t.Context())
	require.NoError(t, err)
	require.False(t, resource.Status.Configuration.Configured)
}
