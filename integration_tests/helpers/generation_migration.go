package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	apiv1alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	connectionschema "github.com/rmorlok/authproxy/internal/schema/resources/connection"
	connectorschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
)

// CreateConnector creates a connector through the API route and returns the
// draft generation response. It is useful when a scenario wants to exercise the
// same management API shape a host application would use.
func (env *IntegrationTestEnv) CreateConnector(
	t *testing.T,
	definition sconfig.ConnectorDefinition,
	labels map[string]string,
	annotations map[string]string,
	opts ...ActorOption,
) connectorschema.Connector {
	t.Helper()

	resource := connectorschema.NewConnector()
	resource.Metadata.Namespace = sconfig.RootNamespace
	resource.Metadata.Labels = labels
	resource.Metadata.Annotations = annotations
	resource.Spec.Definition = definition
	body, err := jsonMarshal(resource)
	require.NoError(t, err)

	w := env.doSignedRequest(t, http.MethodPost, "/api/v1/connectors", body, env.resolveActorOptions(opts))
	require.Equalf(t, http.StatusCreated, w.Code, "create connector failed: %s", w.Body.String())

	var out connectorschema.Connector
	require.NoError(t, jsonUnmarshal(w.Body.Bytes(), &out))
	return out
}

// CreateDraftConnectorGeneration creates the next draft generation for connectorID.
// The server owns the final id/generation/state fields on the definition.
func (env *IntegrationTestEnv) CreateDraftConnectorGeneration(
	t *testing.T,
	connectorID apid.ID,
	definition sconfig.ConnectorDefinition,
	labels map[string]string,
	annotations map[string]string,
	opts ...ActorOption,
) connectorschema.Connector {
	t.Helper()

	req := connectorschema.NewConnector()
	req.Metadata.Namespace = sconfig.RootNamespace
	req.Metadata.Labels = labels
	req.Metadata.Annotations = annotations
	req.Spec.Definition = definition

	body, err := jsonMarshal(req)
	require.NoError(t, err)

	path := fmt.Sprintf("/api/v1/connectors/%s/generations", connectorID)
	w := env.doSignedRequest(t, http.MethodPost, path, body, env.resolveActorOptions(opts))
	require.Equalf(t, http.StatusCreated, w.Code, "create connector generation failed: %s", w.Body.String())

	var out connectorschema.Connector
	require.NoError(t, jsonUnmarshal(w.Body.Bytes(), &out))
	return out
}

// ForceConnectorGenerationState changes the lifecycle state for an existing
// connector generation through the API.
func (env *IntegrationTestEnv) ForceConnectorGenerationState(
	t *testing.T,
	connectorID apid.ID,
	generation uint64,
	state connectorschema.ConnectorReleaseState,
	opts ...ActorOption,
) connectorschema.Connector {
	t.Helper()

	body, err := jsonMarshal(schemaapi.NewConnectorForceStateRequest(
		meta.ObjectReference{
			APIVersion: meta.APIVersionV1Alpha1,
			Kind:       connectorschema.ConnectorKind,
			ID:         connectorID.String(),
			Generation: generation,
		},
		state,
	))
	require.NoError(t, err)

	path := fmt.Sprintf("/api/v1/connectors/%s/generations/%d/_forceState", connectorID, generation)
	w := env.doSignedRequest(t, http.MethodPut, path, body, env.resolveActorOptions(opts))
	require.Equalf(t, http.StatusOK, w.Code, "force connector generation state failed: %s", w.Body.String())

	var out connectorschema.Connector
	require.NoError(t, jsonUnmarshal(w.Body.Bytes(), &out))
	return out
}

// PublishConnectorGeneration creates a draft connector generation and promotes it to
// primary. The previously-primary generation becomes active and can be targeted
// by rollback tests.
func (env *IntegrationTestEnv) PublishConnectorGeneration(
	t *testing.T,
	connectorID apid.ID,
	definition sconfig.ConnectorDefinition,
	labels map[string]string,
	annotations map[string]string,
	opts ...ActorOption,
) connectorschema.Connector {
	t.Helper()

	draft := env.CreateDraftConnectorGeneration(t, connectorID, definition, labels, annotations, opts...)
	return env.ForceConnectorGenerationState(t, connectorID, draft.Metadata.Generation, connectorschema.ConnectorReleaseStatePrimary, opts...)
}

// MigrateConnectionGeneration starts the durable connection-generation migration
// workflow and returns the task response.
func (env *IntegrationTestEnv) MigrateConnectionGeneration(
	t *testing.T,
	connectionID string,
	targetGeneration uint64,
	timeoutSeconds int64,
	opts ...ActorOption,
) schemaapi.ConnectionGenerationMigrationAction {
	t.Helper()

	parsedConnectionID, err := apid.Parse(connectionID)
	require.NoError(t, err)
	connection := env.GetConnection(t, connectionID)
	spec := schemaapi.ConnectionGenerationMigrationSpec{
		ConnectorRef: connectorReference(connection.ConnectorId, targetGeneration),
	}
	if timeoutSeconds > 0 {
		spec.TimeoutSeconds = &timeoutSeconds
	}
	req := schemaapi.ConnectionGenerationMigrationAction{Action: apiv1alpha1.Action[schemaapi.ConnectionGenerationMigrationSpec, schemaapi.ConnectionGenerationMigrationStatus]{
		TypeMeta: meta.NewTypeMeta(schemaapi.ConnectionGenerationMigrationActionKind),
		Metadata: apiv1alpha1.ActionMeta{Target: connectionschema.NewConnectionReference(parsedConnectionID)},
		Spec:     spec,
	}}
	body, err := jsonMarshal(req)
	require.NoError(t, err)

	path := "/api/v1/connections/" + connectionID + "/_migrateGeneration"
	w := env.doSignedRequest(t, http.MethodPost, path, body, env.resolveActorOptions(opts))
	require.Equalf(t, http.StatusOK, w.Code, "migrate connection generation failed: %s", w.Body.String())

	var out schemaapi.ConnectionGenerationMigrationAction
	require.NoError(t, jsonUnmarshal(w.Body.Bytes(), &out))
	require.NoError(t, out.ValidateResponse(schemaapi.ConnectionGenerationMigrationActionKind))
	require.Equal(t, connectionID, out.Metadata.Target.ID)
	require.Equal(t, targetGeneration, out.Spec.ConnectorRef.Generation)
	require.NotNil(t, out.Status)
	require.NotEmpty(t, out.Status.TaskID)
	return out
}

// MigrateConnectionGenerationAndWait starts a migration and waits for the
// workflow-backed task to complete. Call StartCoreWorkflowWorker before using
// this helper.
func (env *IntegrationTestEnv) MigrateConnectionGenerationAndWait(
	t *testing.T,
	connectionID string,
	targetGeneration uint64,
	timeout time.Duration,
	opts ...ActorOption,
) schemaapi.ConnectionGenerationMigrationAction {
	t.Helper()

	timeoutSeconds := int64(timeout.Seconds())
	if timeoutSeconds <= 0 {
		timeoutSeconds = 10
	}
	resp := env.MigrateConnectionGeneration(t, connectionID, targetGeneration, timeoutSeconds, opts...)
	require.NotNil(t, resp.Status)
	RequireWorkflowTaskCompleted(t, env, resp.Status.TaskID, timeout, opts...)
	return resp
}

// SubmitSetupForm submits any schema-defined form step. Auth-method-specific
// helpers can still wrap this for credential forms.
func (env *IntegrationTestEnv) SubmitSetupForm(
	t *testing.T,
	connectionID string,
	stepID string,
	data map[string]any,
	opts ...ActorOption,
) *httptest.ResponseRecorder {
	t.Helper()

	rawData, err := json.Marshal(data)
	require.NoError(t, err)
	body, err := jsonMarshal(connectionSetupSubmitAction(connectionID, stepID, rawData))
	require.NoError(t, err)

	return env.doSignedRequest(t, http.MethodPost, "/api/v1/connections/"+connectionID+"/_submit", body, env.resolveActorOptions(opts))
}

// DecryptConnectionConfiguration reads and decrypts a connection's current
// configuration map. Missing configuration is returned as an empty map.
func (env *IntegrationTestEnv) DecryptConnectionConfiguration(t *testing.T, connectionID string) map[string]any {
	t.Helper()

	conn := env.GetConnection(t, connectionID)
	if conn.EncryptedConfiguration == nil || conn.EncryptedConfiguration.IsZero() {
		return map[string]any{}
	}

	plaintext, err := env.DM.GetEncryptService().DecryptString(context.Background(), *conn.EncryptedConfiguration)
	require.NoError(t, err)

	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(plaintext), &out))
	if out == nil {
		return map[string]any{}
	}
	return out
}

// ListNotifications lists actor-visible notifications through the API. State
// defaults to active when an empty state is supplied.
func (env *IntegrationTestEnv) ListNotifications(
	t *testing.T,
	state schemaapi.NotificationState,
	includeViewed bool,
	opts ...ActorOption,
) []schemaapi.NotificationJson {
	t.Helper()

	q := url.Values{}
	if state != "" {
		q.Set("state", string(state))
	}
	if includeViewed {
		q.Set("includeViewed", "true")
	}
	path := "/api/v1/notifications"
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}

	w := env.doSignedRequest(t, http.MethodGet, path, nil, env.resolveActorOptions(opts))
	require.Equalf(t, http.StatusOK, w.Code, "list notifications failed: %s", w.Body.String())

	var out schemaapi.ListNotificationsResponseJson
	require.NoError(t, jsonUnmarshal(w.Body.Bytes(), &out))
	return out.Items
}

// ListConnectionNotifications filters actor-visible notifications down to one
// connection resource.
func (env *IntegrationTestEnv) ListConnectionNotifications(
	t *testing.T,
	connectionID string,
	state schemaapi.NotificationState,
	includeViewed bool,
	opts ...ActorOption,
) []schemaapi.NotificationJson {
	t.Helper()

	items := env.ListNotifications(t, state, includeViewed, opts...)
	filtered := make([]schemaapi.NotificationJson, 0, len(items))
	for _, n := range items {
		if n.Spec.ResourceRef.Kind == connectionschema.ConnectionKind && n.Spec.ResourceRef.ID == connectionID {
			filtered = append(filtered, n)
		}
	}
	return filtered
}

// RequireSingleActiveConnectionNotification asserts that exactly one active
// notification exists for a connection and that its key has the expected
// high-level suffix, such as "auth_required" or "setup_required".
func (env *IntegrationTestEnv) RequireSingleActiveConnectionNotification(
	t *testing.T,
	connectionID string,
	keySuffix string,
	opts ...ActorOption,
) schemaapi.NotificationJson {
	t.Helper()

	items := env.ListConnectionNotifications(t, connectionID, schemaapi.NotificationStateActive, false, opts...)
	require.Len(t, items, 1, "expected one active connection notification")
	require.Truef(t, strings.HasSuffix(items[0].Spec.Key, ":"+keySuffix),
		"notification key %q should end with %q", items[0].Spec.Key, keySuffix)
	return items[0]
}

// RequireNoActiveConnectionNotifications asserts that the connection has no
// active actor-visible notifications.
func (env *IntegrationTestEnv) RequireNoActiveConnectionNotifications(
	t *testing.T,
	connectionID string,
	opts ...ActorOption,
) {
	t.Helper()
	require.Empty(t, env.ListConnectionNotifications(t, connectionID, schemaapi.NotificationStateActive, false, opts...))
}

// RequireResolvedConnectionNotification asserts that a previously-active
// high-level notification was resolved.
func (env *IntegrationTestEnv) RequireResolvedConnectionNotification(
	t *testing.T,
	connectionID string,
	keySuffix string,
	opts ...ActorOption,
) schemaapi.NotificationJson {
	t.Helper()

	items := env.ListConnectionNotifications(t, connectionID, schemaapi.NotificationStateResolved, true, opts...)
	for _, n := range items {
		if strings.HasSuffix(n.Spec.Key, ":"+keySuffix) {
			require.NotNil(t, n.Status.ResolvedAt, "resolved notification should include status.resolvedAt")
			return n
		}
	}
	require.Failf(t, "resolved notification not found", "connection=%s key_suffix=%s items=%v", connectionID, keySuffix, items)
	return schemaapi.NotificationJson{}
}

// Notification key suffixes used by connection-level required-action
// notifications.
const (
	NotificationKeySuffixAuthRequired  = database.NotificationKeyAuthRequired
	NotificationKeySuffixSetupRequired = database.NotificationKeySetupRequired
)
