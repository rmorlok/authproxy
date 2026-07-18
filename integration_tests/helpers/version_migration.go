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
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/require"
)

// CreateConnector creates a connector through the API route and returns the
// draft version response. It is useful when a scenario wants to exercise the
// same management API shape a host application would use.
func (env *IntegrationTestEnv) CreateConnector(
	t *testing.T,
	definition sconfig.Connector,
	labels map[string]string,
	annotations map[string]string,
	opts ...OAuth2Option,
) schemaapi.ConnectorVersionJson {
	t.Helper()

	body, err := jsonMarshal(schemaapi.CreateConnectorRequestJson{
		Namespace:   sconfig.RootNamespace,
		Definition:  definition,
		Labels:      labels,
		Annotations: annotations,
	})
	require.NoError(t, err)

	w := env.doSignedRequest(t, http.MethodPost, "/api/v1/connectors", body, env.resolveOAuth2Options(opts))
	require.Equalf(t, http.StatusCreated, w.Code, "create connector failed: %s", w.Body.String())

	var out schemaapi.ConnectorVersionJson
	require.NoError(t, jsonUnmarshal(w.Body.Bytes(), &out))
	return out
}

// CreateDraftConnectorVersion creates the next draft version for connectorID.
// The server owns the final id/version/state fields on the definition.
func (env *IntegrationTestEnv) CreateDraftConnectorVersion(
	t *testing.T,
	connectorID apid.ID,
	definition sconfig.Connector,
	labels map[string]string,
	annotations map[string]string,
	opts ...OAuth2Option,
) schemaapi.ConnectorVersionJson {
	t.Helper()

	req := schemaapi.CreateConnectorVersionRequestJson{Definition: &definition}
	if labels != nil {
		req.Labels = &labels
	}
	if annotations != nil {
		req.Annotations = &annotations
	}

	body, err := jsonMarshal(req)
	require.NoError(t, err)

	path := fmt.Sprintf("/api/v1/connectors/%s/versions", connectorID)
	w := env.doSignedRequest(t, http.MethodPost, path, body, env.resolveOAuth2Options(opts))
	require.Equalf(t, http.StatusCreated, w.Code, "create connector version failed: %s", w.Body.String())

	var out schemaapi.ConnectorVersionJson
	require.NoError(t, jsonUnmarshal(w.Body.Bytes(), &out))
	return out
}

// ForceConnectorVersionState changes the lifecycle state for an existing
// connector version through the API.
func (env *IntegrationTestEnv) ForceConnectorVersionState(
	t *testing.T,
	connectorID apid.ID,
	version uint64,
	state schemaapi.ConnectorVersionState,
	opts ...OAuth2Option,
) schemaapi.ConnectorVersionJson {
	t.Helper()

	body, err := jsonMarshal(schemaapi.ForceConnectorVersionStateRequestJson{State: string(state)})
	require.NoError(t, err)

	path := fmt.Sprintf("/api/v1/connectors/%s/versions/%d/_force_state", connectorID, version)
	w := env.doSignedRequest(t, http.MethodPut, path, body, env.resolveOAuth2Options(opts))
	require.Equalf(t, http.StatusOK, w.Code, "force connector version state failed: %s", w.Body.String())

	var out schemaapi.ConnectorVersionJson
	require.NoError(t, jsonUnmarshal(w.Body.Bytes(), &out))
	return out
}

// PublishConnectorVersion creates a draft connector version and promotes it to
// primary. The previously-primary version becomes active and can be targeted
// by rollback tests.
func (env *IntegrationTestEnv) PublishConnectorVersion(
	t *testing.T,
	connectorID apid.ID,
	definition sconfig.Connector,
	labels map[string]string,
	annotations map[string]string,
	opts ...OAuth2Option,
) schemaapi.ConnectorVersionJson {
	t.Helper()

	draft := env.CreateDraftConnectorVersion(t, connectorID, definition, labels, annotations, opts...)
	return env.ForceConnectorVersionState(t, connectorID, draft.Version, schemaapi.ConnectorVersionStatePrimary, opts...)
}

// MigrateConnectionVersion starts the durable connection-version migration
// workflow and returns the task response.
func (env *IntegrationTestEnv) MigrateConnectionVersion(
	t *testing.T,
	connectionID string,
	targetVersion uint64,
	timeoutSeconds int64,
	opts ...OAuth2Option,
) schemaapi.MigrateConnectionVersionResponseJson {
	t.Helper()

	req := schemaapi.MigrateConnectionVersionRequestJson{TargetVersion: targetVersion}
	if timeoutSeconds > 0 {
		req.TimeoutSeconds = &timeoutSeconds
	}
	body, err := jsonMarshal(req)
	require.NoError(t, err)

	path := "/api/v1/connections/" + connectionID + "/_migrate_version"
	w := env.doSignedRequest(t, http.MethodPost, path, body, env.resolveOAuth2Options(opts))
	require.Equalf(t, http.StatusOK, w.Code, "migrate connection version failed: %s", w.Body.String())

	var out schemaapi.MigrateConnectionVersionResponseJson
	require.NoError(t, jsonUnmarshal(w.Body.Bytes(), &out))
	require.Equal(t, connectionID, out.ConnectionId.String())
	require.Equal(t, targetVersion, out.TargetVersion)
	require.NotEmpty(t, out.TaskId)
	return out
}

// MigrateConnectionVersionAndWait starts a migration and waits for the
// workflow-backed task to complete. Call StartCoreWorkflowWorker before using
// this helper.
func (env *IntegrationTestEnv) MigrateConnectionVersionAndWait(
	t *testing.T,
	connectionID string,
	targetVersion uint64,
	timeout time.Duration,
	opts ...OAuth2Option,
) schemaapi.MigrateConnectionVersionResponseJson {
	t.Helper()

	timeoutSeconds := int64(timeout.Seconds())
	if timeoutSeconds <= 0 {
		timeoutSeconds = 10
	}
	resp := env.MigrateConnectionVersion(t, connectionID, targetVersion, timeoutSeconds, opts...)
	RequireWorkflowTaskCompleted(t, env, resp.TaskId, timeout, opts...)
	return resp
}

// SubmitSetupForm submits any schema-defined form step. Auth-method-specific
// helpers can still wrap this for credential forms.
func (env *IntegrationTestEnv) SubmitSetupForm(
	t *testing.T,
	connectionID string,
	stepID string,
	data map[string]any,
	opts ...OAuth2Option,
) *httptest.ResponseRecorder {
	t.Helper()

	rawData, err := json.Marshal(data)
	require.NoError(t, err)
	body, err := jsonMarshal(iface.SubmitConnectionRequest{
		StepId: stepID,
		Data:   rawData,
	})
	require.NoError(t, err)

	return env.doSignedRequest(t, http.MethodPost, "/api/v1/connections/"+connectionID+"/_submit", body, env.resolveOAuth2Options(opts))
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
	opts ...OAuth2Option,
) []schemaapi.NotificationJson {
	t.Helper()

	q := url.Values{}
	if state != "" {
		q.Set("state", string(state))
	}
	if includeViewed {
		q.Set("include_viewed", "true")
	}
	path := "/api/v1/notifications"
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}

	w := env.doSignedRequest(t, http.MethodGet, path, nil, env.resolveOAuth2Options(opts))
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
	opts ...OAuth2Option,
) []schemaapi.NotificationJson {
	t.Helper()

	id, err := apid.Parse(connectionID)
	require.NoError(t, err)

	items := env.ListNotifications(t, state, includeViewed, opts...)
	filtered := make([]schemaapi.NotificationJson, 0, len(items))
	for _, n := range items {
		if n.ResourceType == "connection" && n.ResourceId == id {
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
	opts ...OAuth2Option,
) schemaapi.NotificationJson {
	t.Helper()

	items := env.ListConnectionNotifications(t, connectionID, schemaapi.NotificationStateActive, false, opts...)
	require.Len(t, items, 1, "expected one active connection notification")
	require.Truef(t, strings.HasSuffix(items[0].Key, ":"+keySuffix),
		"notification key %q should end with %q", items[0].Key, keySuffix)
	return items[0]
}

// RequireNoActiveConnectionNotifications asserts that the connection has no
// active actor-visible notifications.
func (env *IntegrationTestEnv) RequireNoActiveConnectionNotifications(
	t *testing.T,
	connectionID string,
	opts ...OAuth2Option,
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
	opts ...OAuth2Option,
) schemaapi.NotificationJson {
	t.Helper()

	items := env.ListConnectionNotifications(t, connectionID, schemaapi.NotificationStateResolved, true, opts...)
	for _, n := range items {
		if strings.HasSuffix(n.Key, ":"+keySuffix) {
			require.NotNil(t, n.ResolvedAt, "resolved notification should include resolved_at")
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
