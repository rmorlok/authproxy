package tools

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/connection"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// authoredToolForTest is a complete authorable resource without server-owned
// identity or status, so lifecycle tests can add those fields independently.
func authoredToolForTest() *Tool {
	tool := NewTool()
	tool.Metadata.Namespace = "root.product.user-1"
	tool.Spec = ToolSpec{
		ConnectionRef: connection.NewConnectionReference(apid.ID("cxn_01example0000001")),
		ToolDefinition: ToolDefinition{
			Description: "List records", Verbs: []string{"tool:records.list"},
			InputSchema: common.RawJSON(`{"type":"object"}`),
			ProxyHTTP:   &ProxyHTTP{Method: "GET", URL: "https://{{cfg.host}}/records"},
		},
	}
	return tool
}

// storedToolForTest represents an already-published revision; a schema-only
// patch must retain this status until the publication layer commits a revision.
func storedToolForTest() *Tool {
	tool := authoredToolForTest()
	tool.Metadata.ID = "tol_01example0000001"
	tool.Metadata.Name = "list-records"
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	tool.Metadata.CreatedAt, tool.Metadata.UpdatedAt = &now, &now
	tool.Status = &ToolStatus{Revision: 7, Conditions: []ToolCondition{{
		Type: "Ready", Status: meta.ConditionTrue, ObservedRevision: 7, LastTransitionTime: now,
	}}}
	return tool
}

// managedToolForTest associates a generated Tool with a parent namespace's
// ToolSet and its applied generation; source keys remain opaque identity data.
func managedToolForTest() *Tool {
	tool := storedToolForTest()
	tool.Status.ManagedBy = &ToolManagedBy{
		ToolSetRef: meta.ObjectReference{
			APIVersion: meta.APIVersionV1Alpha1, Kind: "ToolSet", ID: "tls_01example0000001",
			Namespace: "root.product", Generation: 3,
		},
		SourceKey: "GET /records/{id}",
	}
	return tool
}

// TestToolResourceLifecycle separates authored requests from resolved stored
// resources without assigning a fake revision or synthesizing readiness.
func TestToolResourceLifecycle(t *testing.T) {
	authored := authoredToolForTest()
	for _, mode := range []meta.ValidationMode{meta.ValidationModeCreate, meta.ValidationModeConfig, meta.ValidationModeUpdate} {
		require.NoError(t, authored.ValidateFor(mode, nil))
	}
	require.NoError(t, authored.Validate(nil))
	for _, mode := range []meta.ValidationMode{meta.ValidationModePersistence, meta.ValidationModeResponse} {
		err := authored.ValidateFor(mode, nil)
		require.ErrorContains(t, err, "metadata.id")
		require.ErrorContains(t, err, "metadata.name")
		require.ErrorContains(t, err, "status")
		require.NoError(t, storedToolForTest().ValidateFor(mode, nil))
		require.NoError(t, managedToolForTest().ValidateFor(mode, nil))
	}
	stored := storedToolForTest()
	require.ErrorContains(t, stored.ValidateFor(meta.ValidationModeCreate, nil), "server-owned on create")
	for _, mode := range []meta.ValidationMode{meta.ValidationModeConfig, meta.ValidationModeUpdate} {
		require.ErrorContains(t, stored.ValidateFor(mode, nil), "status")
	}
	stored.Status.Conditions = nil
	require.NoError(t, stored.ValidateFor(meta.ValidationModeResponse, nil))
	require.ErrorContains(t, stored.ValidateFor("unsupported", nil), "unknown metadata validation mode")
	require.Error(t, (*Tool)(nil).Validate(nil))
}

// TestToolSubcontractValidation keeps nil and unresolved contract errors at the
// caller's field path, including the explicit ToolSpec validation boundary.
func TestToolSubcontractValidation(t *testing.T) {
	vc := &common.ValidationContext{Path: "candidate"}
	requireValidationPath(t, (*ToolSpec)(nil).Validate(vc), "candidate")
	requireValidationPath(t, (*ToolStatus)(nil).ValidateForNamespace("root", vc), "candidate")
	requireValidationPath(t, (*ToolManagedBy)(nil).ValidateForNamespace("root", vc), "candidate")
	spec := authoredToolForTest().Spec
	spec.ConnectionRef = meta.ObjectReference{}
	require.ErrorContains(t, spec.Validate(vc), "candidate.connectionRef")
	for _, id := range []string{"", "unknown", "cxn_example"} {
		require.Error(t, ValidateID(id))
		tool := NewTool()
		tool.Metadata.ID = id
		require.Equal(t, apid.Nil, tool.GetId())
	}
	require.Nil(t, (*Tool)(nil).ApplyCreateDefaults(apid.ID("tol_example")))
	tool := storedToolForTest()
	require.Equal(t, tool.Metadata.Name, tool.ApplyCreateDefaults(tool.GetId()).Metadata.Name)
}

// TestToolResourceValidation checks identity and namespace boundaries in the
// direct Go API as well as delegation to full authored-definition validation.
func TestToolResourceValidation(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*Tool)
		field  string
	}{
		{"kind", func(v *Tool) { v.Kind = "Actor" }, "kind"},
		{"version", func(v *Tool) { v.APIVersion = "authproxy.net/v2" }, "apiVersion"},
		{"namespace absent", func(v *Tool) { v.Metadata.Namespace = "" }, "metadata.namespace"},
		{"namespace invalid", func(v *Tool) { v.Metadata.Namespace = "elsewhere" }, "metadata.namespace"},
		{"tool prefix", func(v *Tool) { v.Metadata.ID = "cxn_01example0000001" }, "metadata.id"},
		{"generation", func(v *Tool) { v.Metadata.Generation = 1 }, "metadata.generation"},
		{"connection absent", func(v *Tool) { v.Spec.ConnectionRef = meta.ObjectReference{} }, "spec.connectionRef"},
		{"connection kind", func(v *Tool) { v.Spec.ConnectionRef.Kind = "Actor" }, "spec.connectionRef.kind"},
		{"connection version", func(v *Tool) { v.Spec.ConnectionRef.APIVersion = "authproxy.net/v2" }, "spec.connectionRef.apiVersion"},
		{"connection prefix", func(v *Tool) { v.Spec.ConnectionRef.ID = "tol_01example0000001" }, "spec.connectionRef.id"},
		{"connection generation", func(v *Tool) { v.Spec.ConnectionRef.Generation = 1 }, "spec.connectionRef.generation"},
		{"cross namespace", func(v *Tool) { v.Spec.ConnectionRef.Namespace = "root.product.user-2" }, "spec.connectionRef.namespace"},
		{"unresolved stored ref", func(v *Tool) {
			v.Spec.ConnectionRef.ID = ""
			v.Spec.ConnectionRef.Namespace = v.Metadata.Namespace
			v.Spec.ConnectionRef.Name = "account"
		}, "spec.connectionRef.id"},
		{"definition", func(v *Tool) { v.Spec.Verbs = nil }, "spec.verbs"},
	} {
		t.Run(test.name, func(t *testing.T) {
			tool := storedToolForTest()
			test.change(tool)
			require.ErrorContains(t, tool.ValidateFor(meta.ValidationModePersistence, &common.ValidationContext{Path: "resource"}), "resource."+test.field)
		})
	}
	tool := authoredToolForTest()
	tool.Spec.ConnectionRef.ID = ""
	tool.Spec.ConnectionRef.Namespace = tool.Metadata.Namespace
	tool.Spec.ConnectionRef.Name = "account"
	require.NoError(t, tool.Validate(nil), "authoring may use a same-namespace name resolved by core later")
}

// TestToolStatusValidation bounds revision diagnostics and generated ownership
// without requiring a condition to have observed the newest published revision.
func TestToolStatusValidation(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*Tool)
		field  string
	}{
		{"missing revision", func(v *Tool) { v.Status.Revision = 0 }, "status.revision"},
		{"future observation", func(v *Tool) { v.Status.Conditions[0].ObservedRevision = 8 }, "status.conditions[0].observedRevision"},
		{"condition type", func(v *Tool) { v.Status.Conditions[0].Type = "" }, "status.conditions[0].type"},
		{"condition status", func(v *Tool) { v.Status.Conditions[0].Status = "Maybe" }, "status.conditions[0].status"},
		{"condition time", func(v *Tool) { v.Status.Conditions[0].LastTransitionTime = time.Time{} }, "status.conditions[0].lastTransitionTime"},
		{"duplicate condition", func(v *Tool) { v.Status.Conditions = append(v.Status.Conditions, v.Status.Conditions[0]) }, "status.conditions[1].type"},
		{"owner kind", func(v *Tool) { v.Status.ManagedBy.ToolSetRef.Kind = "Tool" }, "status.managedBy.toolSetRef.kind"},
		{"owner version", func(v *Tool) { v.Status.ManagedBy.ToolSetRef.APIVersion = "authproxy.net/v2" }, "status.managedBy.toolSetRef.apiVersion"},
		{"owner prefix", func(v *Tool) { v.Status.ManagedBy.ToolSetRef.ID = v.Metadata.ID }, "status.managedBy.toolSetRef.id"},
		{"unresolved owner", func(v *Tool) { v.Status.ManagedBy.ToolSetRef.ID = ""; v.Status.ManagedBy.ToolSetRef.Name = "source" }, "status.managedBy.toolSetRef.id"},
		{"owner generation", func(v *Tool) { v.Status.ManagedBy.ToolSetRef.Generation = 0 }, "status.managedBy.toolSetRef.generation"},
		{"owner sibling", func(v *Tool) { v.Status.ManagedBy.ToolSetRef.Namespace = "root.product.user-2" }, "status.managedBy.toolSetRef.namespace"},
		{"owner string prefix", func(v *Tool) { v.Status.ManagedBy.ToolSetRef.Namespace = "root.prod" }, "status.managedBy.toolSetRef.namespace"},
		{"empty source key", func(v *Tool) { v.Status.ManagedBy.SourceKey = " \n" }, "status.managedBy.sourceKey"},
	} {
		t.Run(test.name, func(t *testing.T) {
			tool := managedToolForTest()
			test.change(tool)
			require.ErrorContains(t, tool.ValidateFor(meta.ValidationModeResponse, nil), test.field)
		})
	}
	for _, ownerNamespace := range []string{"", "root", "root.product", "root.product.user-1"} {
		tool := managedToolForTest()
		tool.Status.ManagedBy.ToolSetRef.Namespace = ownerNamespace
		tool.Status.Conditions[0].ObservedRevision = 0
		require.NoError(t, tool.ValidateFor(meta.ValidationModePersistence, nil))
		require.Equal(t, "GET /records/{id}", tool.Status.ManagedBy.SourceKey)
	}
}

// TestToolUpdateValidation preserves identity and server state while allowing
// ordinary standalone spec changes, including a same-namespace connection.
func TestToolUpdateValidation(t *testing.T) {
	before := storedToolForTest()
	after := before.Clone()
	after.Metadata.Name = "renamed"
	after.Spec.Description = "New description"
	after.Spec.ConnectionRef.ID = "cxn_02example0000001"
	require.NoError(t, ValidateUpdate(before, after, nil))
	for _, test := range []struct {
		name   string
		change func(*Tool)
		field  string
	}{
		{"id", func(v *Tool) { v.Metadata.ID = "tol_02example0000001" }, "metadata.id"},
		{"namespace", func(v *Tool) { v.Metadata.Namespace = "root.other" }, "metadata.namespace"},
		{"createdAt", func(v *Tool) { *v.Metadata.CreatedAt = v.Metadata.CreatedAt.Add(time.Second) }, "metadata.createdAt"},
		{"kind", func(v *Tool) { v.Kind = "ToolSet" }, "kind"},
		{"revision", func(v *Tool) { v.Status.Revision++ }, "status"},
		{"removed status", func(v *Tool) { v.Status = nil }, "status"},
		{"added ownership", func(v *Tool) { v.Status.ManagedBy = managedToolForTest().Status.ManagedBy }, "status"},
	} {
		t.Run(test.name, func(t *testing.T) {
			after := before.Clone()
			test.change(after)
			require.ErrorContains(t, ValidateUpdate(before, after, nil), test.field)
		})
	}
	managed := managedToolForTest()
	require.ErrorContains(t, ValidateUpdate(managed, managed.Clone(), nil), "tls_01example0000001")
	require.Error(t, ValidateUpdate(nil, after, nil))
	require.Error(t, ValidateUpdate(before, nil, nil))
}

// TestToolResourceRoundTrip checks that definition fields stay flat in spec
// and that revision diagnostics never become Connector-style generations.
func TestToolResourceRoundTrip(t *testing.T) {
	tool := managedToolForTest()
	for _, format := range []struct {
		name   string
		encode func(any) ([]byte, error)
		decode func([]byte, any) error
	}{
		{"JSON", json.Marshal, util.DecodeJSONStrict},
		{"YAML", yaml.Marshal, util.DecodeYAMLStrict},
	} {
		t.Run(format.name, func(t *testing.T) {
			data, err := format.encode(tool)
			require.NoError(t, err)
			require.NotContains(t, string(data), "ToolDefinition")
			require.NotContains(t, string(data), "observedGeneration")
			var decoded Tool
			require.NoError(t, format.decode(data, &decoded))
			require.NoError(t, decoded.ValidateFor(meta.ValidationModeResponse, nil))
			require.Equal(t, tool, &decoded)
		})
	}
	id := apid.ID("tol_01example0000001")
	require.Equal(t, id, tool.GetId())
	require.NoError(t, ValidateID(tool.Metadata.ID))
	ref := NewToolReference(id)
	require.Equal(t, meta.APIVersionV1Alpha1, ref.APIVersion)
	require.Equal(t, ToolKind, ref.Kind)
	require.Equal(t, id.String(), ref.ID)
	require.Zero(t, ref.Generation)
	var missing *Tool
	require.Equal(t, apid.Nil, missing.GetId())
	require.Nil(t, missing.Clone())
	authored := authoredToolForTest()
	created := authored.ApplyCreateDefaults(id)
	require.Empty(t, created.Metadata.ID)
	require.Equal(t, id.String(), string(created.Metadata.Name))
	require.Empty(t, authored.Metadata.ID)
	require.Nil(t, created.Status)
}

// cloneToolForTest intentionally includes competing body forms and malformed
// raw data: cloning must preserve validation inputs even before admission.
func cloneToolForTest(t *testing.T) *Tool {
	t.Helper()
	tool := managedToolForTest()
	tool.Metadata.Labels = map[string]string{"team": "platform"}
	tool.Metadata.Annotations = map[string]string{"owner": "alice"}
	require.NoError(t, json.Unmarshal(definitionJSON(`
		"outputSchema":false,
		"hints":{"readOnly":false,"destructive":true,"idempotent":false},
		"limits":{"timeoutMillis":1,"maxRequests":2,"maxInputBytes":3,"maxOutputBytes":4,"maxResponseBytes":5,"maxEncodedResultBytes":6,"maxStackDepth":7},
		"javascript":"code",
		"proxyHttp":{
			"method":"POST","url":"https://example.test",
			"headers":{"X-Test":"original"},"query":{"q":[1]},"bodyJson":null,
			"bodyTemplate":{"mediaType":"text/plain","template":"original"},"bodyRaw":"AA==",
			"form":{"fields":{"field":false}},
			"multipart":{"parts":[{"name":"text","text":"original"},{"name":"file","bodyRaw":"AA=="}]},
			"response":{"transformJavascript":"code","errors":[{"statuses":[404],"statusClass":4,"retryable":false,"code":"MISSING","message":"Missing"}]}
		}`), &tool.Spec.ToolDefinition))
	tool.Spec.InputSchema = common.RawJSON(" { broken ")
	tool.Spec.ProxyHTTP.nullBodyFields = []string{"bodyRaw"}
	return tool
}

// TestToolCloneDetachesNestedState ensures candidate edits cannot alter the
// original resource, its executable bytes, or recorded controller ownership.
func TestToolCloneDetachesNestedState(t *testing.T) {
	for name, change := range map[string]func(*Tool){
		"labels":                func(v *Tool) { v.Metadata.Labels["team"] = "other" },
		"annotations":           func(v *Tool) { v.Metadata.Annotations["owner"] = "other" },
		"createdAt":             func(v *Tool) { *v.Metadata.CreatedAt = v.Metadata.CreatedAt.Add(time.Second) },
		"updatedAt":             func(v *Tool) { *v.Metadata.UpdatedAt = v.Metadata.UpdatedAt.Add(time.Second) },
		"verbs":                 func(v *Tool) { v.Spec.Verbs[0] = "tool:other" },
		"input bytes":           func(v *Tool) { v.Spec.InputSchema[0] = '[' },
		"output bytes":          func(v *Tool) { v.Spec.OutputSchema[0] = 't' },
		"javascript":            func(v *Tool) { *v.Spec.Javascript = "other" },
		"readOnly":              func(v *Tool) { *v.Spec.Hints.ReadOnly = true },
		"destructive":           func(v *Tool) { *v.Spec.Hints.Destructive = false },
		"idempotent":            func(v *Tool) { *v.Spec.Hints.Idempotent = true },
		"timeout":               func(v *Tool) { *v.Spec.Limits.TimeoutMillis++ },
		"requests":              func(v *Tool) { *v.Spec.Limits.MaxRequests++ },
		"input limit":           func(v *Tool) { *v.Spec.Limits.MaxInputBytes++ },
		"output limit":          func(v *Tool) { *v.Spec.Limits.MaxOutputBytes++ },
		"response limit":        func(v *Tool) { *v.Spec.Limits.MaxResponseBytes++ },
		"encoded limit":         func(v *Tool) { *v.Spec.Limits.MaxEncodedResultBytes++ },
		"stack limit":           func(v *Tool) { *v.Spec.Limits.MaxStackDepth++ },
		"method":                func(v *Tool) { v.Spec.ProxyHTTP.Method = "PUT" },
		"headers":               func(v *Tool) { v.Spec.ProxyHTTP.Headers["X-Test"] = "other" },
		"query":                 func(v *Tool) { v.Spec.ProxyHTTP.Query["q"][1] = '2' },
		"body":                  func(v *Tool) { v.Spec.ProxyHTTP.BodyJSON[0] = 't' },
		"template":              func(v *Tool) { v.Spec.ProxyHTTP.BodyTemplate.Template = "other" },
		"raw":                   func(v *Tool) { *v.Spec.ProxyHTTP.BodyRaw = "AQ==" },
		"form":                  func(v *Tool) { v.Spec.ProxyHTTP.Form.Fields["field"][0] = 't' },
		"multipart metadata":    func(v *Tool) { v.Spec.ProxyHTTP.Multipart.Parts[0].Name = "other" },
		"multipart text":        func(v *Tool) { *v.Spec.ProxyHTTP.Multipart.Parts[0].Text = "other" },
		"multipart raw":         func(v *Tool) { *v.Spec.ProxyHTTP.Multipart.Parts[1].BodyRaw = "AQ==" },
		"transform":             func(v *Tool) { *v.Spec.ProxyHTTP.Response.TransformJavascript = "other" },
		"status list":           func(v *Tool) { v.Spec.ProxyHTTP.Response.Errors[0].Statuses[0] = 400 },
		"status class":          func(v *Tool) { *v.Spec.ProxyHTTP.Response.Errors[0].StatusClass = 5 },
		"retryable":             func(v *Tool) { *v.Spec.ProxyHTTP.Response.Errors[0].Retryable = true },
		"error mapping":         func(v *Tool) { v.Spec.ProxyHTTP.Response.Errors[0].Code = "OTHER" },
		"null validation state": func(v *Tool) { v.Spec.ProxyHTTP.nullBodyFields[0] = "form" },
		"revision":              func(v *Tool) { v.Status.Revision++ },
		"condition":             func(v *Tool) { v.Status.Conditions[0].ObservedRevision = 1 },
		"owner":                 func(v *Tool) { v.Status.ManagedBy.SourceKey = "other" },
	} {
		t.Run(name, func(t *testing.T) {
			original := cloneToolForTest(t)
			clone := original.Clone()
			require.Equal(t, original, clone)
			change(clone)
			require.NotEqual(t, original, clone)
			require.Equal(t, cloneToolForTest(t), original, "candidate mutation must not affect source")
		})
	}
	var definition *ToolDefinition
	require.Nil(t, definition.Clone())
}
