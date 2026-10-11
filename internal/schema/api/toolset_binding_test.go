package api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/common"
	connectionschema "github.com/rmorlok/authproxy/internal/schema/resources/connection"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	toolsetschema "github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// toolSetBindingForTest supplies a pending assignment in a connection owner's
// namespace, before any ToolSet generation or MCP inventory has been installed.
func toolSetBindingForTest() *ConnectionToolSetBinding {
	return NewConnectionToolSetBinding("root.product.user-1",
		connectionschema.NewConnectionReference(apid.ID("cxn_01example0000001")),
		toolsetschema.NewToolSetReference(apid.ID("tls_01example0000001")))
}

// mcpCatalogForTest keeps attempt start separate from successful completion.
func mcpCatalogForTest() *MCPCatalogStatus {
	attempt := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	return &MCPCatalogStatus{
		LastAttemptAt: &attempt,
		LastComplete: &MCPCatalogSnapshot{
			Revision: 9, SourceDigest: "sha256:example", LastSuccessAt: attempt.Add(time.Second),
			Freshness: MCPCatalogFreshnessFresh, ContextMatches: meta.ConditionTrue,
		},
	}
}

// TestConnectionToolSetBindingLifecycle distinguishes pending installation from
// failed migration and retained catalogs that may no longer be executable.
func TestConnectionToolSetBindingLifecycle(t *testing.T) {
	binding := toolSetBindingForTest()
	require.NotNil(t, binding.Status)
	require.Nil(t, binding.Status.AppliedGeneration)
	require.NoError(t, binding.Validate(nil))

	binding.Status.Catalog = &MCPCatalogStatus{
		LastAttemptAt: mcpCatalogForTest().LastAttemptAt,
		Diagnostics: []toolsetschema.SourceDiagnostic{{
			Severity: toolsetschema.SourceDiagnosticSeverityError, Code: "enumeration-failed", Message: "The catalog is incomplete.",
		}},
	}
	require.NoError(t, binding.Validate(nil), "first discovery can fail before an inventory is installed")
	require.Nil(t, binding.Status.AppliedGeneration)
	require.Nil(t, binding.Status.Catalog.LastComplete)

	binding.Status.AppliedGeneration = util.ToPtr(uint64(2))
	binding.Status.Catalog = mcpCatalogForTest()
	for _, target := range []uint64{5, 1} {
		binding.Status.Conditions = []meta.Condition{{
			Type: ToolSetBindingConditionMigrationSucceeded, Status: meta.ConditionFalse,
			ObservedGeneration: target, LastTransitionTime: *binding.Status.Catalog.LastAttemptAt,
			Reason: "CompilationFailed", Message: "The previous inventory remains installed.",
		}}
		require.NoError(t, binding.Validate(nil), "failed upgrade and rollback both retain the installed generation")
		require.Equal(t, uint64(2), *binding.Status.AppliedGeneration)
	}

	for _, freshness := range []MCPCatalogFreshness{MCPCatalogFreshnessFresh, MCPCatalogFreshnessStale} {
		for _, context := range []meta.ConditionStatus{meta.ConditionTrue, meta.ConditionFalse, meta.ConditionUnknown} {
			binding.Status.Catalog.LastComplete.Freshness = freshness
			binding.Status.Catalog.LastComplete.ContextMatches = context
			require.NoError(t, binding.Validate(nil), "observations do not grant execution")
			require.Equal(t, uint64(9), binding.Status.Catalog.LastComplete.Revision)
		}
	}
}

// TestConnectionToolSetBindingValidation requires resolved logical identities
// and scoped namespaces without inventing a separate binding resource identity.
func TestConnectionToolSetBindingValidation(t *testing.T) {
	require.Error(t, (*ConnectionToolSetBinding)(nil).Validate(nil))
	for _, test := range []struct {
		name   string
		change func(*ConnectionToolSetBinding)
		field  string
	}{
		{"wrong version", func(b *ConnectionToolSetBinding) { b.APIVersion = "authproxy.net/v2" }, "apiVersion"},
		{"wrong kind", func(b *ConnectionToolSetBinding) { b.Kind = "ToolSet" }, "kind"},
		{"missing namespace", func(b *ConnectionToolSetBinding) { b.Metadata.Namespace = "" }, "metadata.namespace"},
		{"invalid namespace", func(b *ConnectionToolSetBinding) { b.Metadata.Namespace = "root..product" }, "metadata.namespace"},
		{"binding id", func(b *ConnectionToolSetBinding) { b.Metadata.ID = "tls_01example0000001" }, "metadata"},
		{"binding name", func(b *ConnectionToolSetBinding) { b.Metadata.Name = "binding" }, "metadata"},
		{"binding generation", func(b *ConnectionToolSetBinding) { b.Metadata.Generation = 2 }, "metadata"},
		{"missing status", func(b *ConnectionToolSetBinding) { b.Status = nil }, "status"},
		{"zero applied generation", func(b *ConnectionToolSetBinding) { b.Status.AppliedGeneration = util.ToPtr(uint64(0)) }, "status.appliedGeneration"},
		{"named connection", func(b *ConnectionToolSetBinding) {
			b.Spec.ConnectionRef.ID, b.Spec.ConnectionRef.Name, b.Spec.ConnectionRef.Namespace = "", "calendar", b.Metadata.Namespace
		}, "spec.connectionRef.id"},
		{"named ToolSet", func(b *ConnectionToolSetBinding) {
			b.Spec.ToolSetRef.ID, b.Spec.ToolSetRef.Name, b.Spec.ToolSetRef.Namespace = "", "calendar-tools", "root.product"
		}, "spec.toolSetRef.id"},
		{"connection ID type", func(b *ConnectionToolSetBinding) { b.Spec.ConnectionRef.ID = b.Spec.ToolSetRef.ID }, "spec.connectionRef.id"},
		{"ToolSet ID type", func(b *ConnectionToolSetBinding) { b.Spec.ToolSetRef.ID = b.Spec.ConnectionRef.ID }, "spec.toolSetRef.id"},
		{"connection kind", func(b *ConnectionToolSetBinding) { b.Spec.ConnectionRef.Kind = "ToolSet" }, "spec.connectionRef.kind"},
		{"ToolSet version", func(b *ConnectionToolSetBinding) { b.Spec.ToolSetRef.APIVersion = "authproxy.net/v2" }, "spec.toolSetRef.apiVersion"},
		{"connection generation", func(b *ConnectionToolSetBinding) { b.Spec.ConnectionRef.Generation = 1 }, "spec.connectionRef.generation"},
		{"ToolSet generation", func(b *ConnectionToolSetBinding) { b.Spec.ToolSetRef.Generation = 2 }, "spec.toolSetRef.generation"},
		{"connection namespace", func(b *ConnectionToolSetBinding) { b.Spec.ConnectionRef.Namespace = "root.product" }, "spec.connectionRef.namespace"},
		{"descendant ToolSet", func(b *ConnectionToolSetBinding) { b.Spec.ToolSetRef.Namespace = "root.product.user-1.child" }, "spec.toolSetRef.namespace"},
		{"sibling ToolSet", func(b *ConnectionToolSetBinding) { b.Spec.ToolSetRef.Namespace = "root.product.user-2" }, "spec.toolSetRef.namespace"},
		{"namespace prefix is not ancestor", func(b *ConnectionToolSetBinding) { b.Spec.ToolSetRef.Namespace = "root.prod" }, "spec.toolSetRef.namespace"},
		{"snapshot without generation", func(b *ConnectionToolSetBinding) { b.Status.Catalog = mcpCatalogForTest() }, "status.appliedGeneration"},
	} {
		t.Run(test.name, func(t *testing.T) {
			binding := toolSetBindingForTest()
			test.change(binding)
			require.ErrorContains(t, binding.Validate(&common.ValidationContext{Path: "response"}), "response."+test.field)
		})
	}

	for _, namespace := range []string{"", "root", "root.product", "root.product.user-1"} {
		binding := toolSetBindingForTest()
		binding.Spec.ConnectionRef.Namespace, binding.Spec.ConnectionRef.Name = binding.Metadata.Namespace, "calendar"
		binding.Spec.ToolSetRef.Namespace, binding.Spec.ToolSetRef.Name = namespace, "calendar-tools"
		binding.Metadata.Labels, binding.Metadata.Annotations = map[string]string{"team": "support"}, map[string]string{"note": "managed"}
		binding.Metadata.CreatedAt = mcpCatalogForTest().LastAttemptAt
		require.NoError(t, binding.Validate(nil), namespace)
	}
}

// TestConnectionToolSetBindingConditions checks shared condition semantics and
// uniqueness while leaving observed generations and condition names extensible.
func TestConnectionToolSetBindingConditions(t *testing.T) {
	binding := toolSetBindingForTest()
	binding.Status.Conditions = []meta.Condition{{
		Type: ToolSetBindingConditionInstalled, Status: meta.ConditionUnknown,
		LastTransitionTime: *mcpCatalogForTest().LastAttemptAt, Reason: "WaitingForPrimary",
	}}
	require.NoError(t, binding.Validate(nil))
	condition := binding.Status.Conditions[0]
	binding.Status.Conditions = append(binding.Status.Conditions, condition)
	require.ErrorContains(t, binding.Validate(nil), "conditions[1].type")
	binding.Status.Conditions[1].Type = "FutureObservation"
	binding.Status.Conditions[1].ObservedGeneration = 100
	require.NoError(t, binding.Validate(nil))
	binding.Status.Conditions[1].Status = "ready"
	require.ErrorContains(t, binding.Validate(nil), "conditions[1].status")
	binding.Status.Conditions[1] = condition
	binding.Status.Conditions[1].Type = ToolSetBindingConditionReady
	binding.Status.Conditions[1].LastTransitionTime = time.Time{}
	require.ErrorContains(t, binding.Validate(nil), "conditions[1].lastTransitionTime")
}

// TestMCPCatalogValidation retains failure diagnostics separately from completed
// snapshots, requiring only observations that the status can truthfully report.
func TestMCPCatalogValidation(t *testing.T) {
	require.NoError(t, (*MCPCatalogStatus)(nil).Validate(nil))
	require.NoError(t, (&MCPCatalogStatus{}).Validate(nil))
	require.Error(t, (*MCPCatalogSnapshot)(nil).Validate(nil))
	for _, test := range []struct {
		change func(*MCPCatalogStatus)
		field  string
	}{
		{func(c *MCPCatalogStatus) { c.LastAttemptAt = nil }, "lastAttemptAt"},
		{func(c *MCPCatalogStatus) { c.LastAttemptAt = &time.Time{} }, "lastAttemptAt"},
		{func(c *MCPCatalogStatus) { c.LastComplete.Revision = 0 }, "lastComplete.revision"},
		{func(c *MCPCatalogStatus) { c.LastComplete.SourceDigest = " \n" }, "lastComplete.sourceDigest"},
		{func(c *MCPCatalogStatus) { c.LastComplete.LastSuccessAt = time.Time{} }, "lastComplete.lastSuccessAt"},
		{func(c *MCPCatalogStatus) { c.LastComplete.Freshness = "unknown" }, "lastComplete.freshness"},
		{func(c *MCPCatalogStatus) { c.LastComplete.ContextMatches = "true" }, "lastComplete.contextMatches"},
		{func(c *MCPCatalogStatus) { c.Diagnostics = []toolsetschema.SourceDiagnostic{{}} }, "diagnostics[0].code"},
	} {
		catalog := mcpCatalogForTest()
		test.change(catalog)
		require.ErrorContains(t, catalog.Validate(&common.ValidationContext{Path: "catalog"}), "catalog."+test.field)
	}
	require.NoError(t, mcpCatalogForTest().Validate(nil), "attempt starts before successful completion")
}

// TestConnectionToolSetBindingRoundTrip preserves management observations in
// both wire formats and rejects fields outside the public projection contract.
func TestConnectionToolSetBindingRoundTrip(t *testing.T) {
	for _, format := range []struct {
		name   string
		encode func(any) ([]byte, error)
		decode func([]byte, any) error
	}{
		{"JSON", json.Marshal, util.DecodeJSONStrict},
		{"YAML", yaml.Marshal, util.DecodeYAMLStrict},
	} {
		t.Run(format.name, func(t *testing.T) {
			binding := toolSetBindingForTest()
			binding.Status.AppliedGeneration, binding.Status.Catalog = util.ToPtr(uint64(2)), mcpCatalogForTest()
			binding.Status.Catalog.Diagnostics = []toolsetschema.SourceDiagnostic{{
				Severity: toolsetschema.SourceDiagnosticSeverityWarning, Code: "provider-warning", Message: "Some tools require setup.",
				Location: &toolsetschema.SourceDiagnosticLocation{JSONPointer: util.ToPtr("")},
			}}
			list := NewConnectionToolSetBindingList([]ConnectionToolSetBinding{*binding}, "next+/=")
			data, err := format.encode(list)
			require.NoError(t, err)
			var decoded ConnectionToolSetBindingList
			require.NoError(t, format.decode(data, &decoded))
			require.Equal(t, list, decoded)
			require.NoError(t, decoded.Validate())

			for _, input := range []string{
				`{"spec":{"endpoint":"https://private.example.test"}}`,
				`{"status":{"catalog":{"credentials":{}}}}`,
				`{"status":{"catalog":{"lastComplete":{"contextFingerprint":"private"}}}}`,
				`{"status":{"catalog":{"diagnostics":[{"unknown":1}]}}}`,
			} {
				var invalid ConnectionToolSetBinding
				require.Error(t, format.decode([]byte(input), &invalid), input)
			}
		})
	}
}

// TestConnectionToolSetBindingLists validates page and item identities while
// preserving opaque cursors and normalizing constructor input to an empty array.
func TestConnectionToolSetBindingLists(t *testing.T) {
	require.Error(t, (*ConnectionToolSetBindingList)(nil).Validate())
	list := NewConnectionToolSetBindingList(nil, "next+/=")
	require.NotNil(t, list.Items)
	require.Empty(t, list.Items)
	require.Equal(t, "next+/=", list.Metadata.Continue)
	require.NoError(t, list.Validate())
	list.Items = []ConnectionToolSetBinding{*toolSetBindingForTest()}
	list.Metadata.RemainingItemCount = util.ToPtr(int64(4))
	require.NoError(t, list.Validate())
	list.Metadata.RemainingItemCount = util.ToPtr(int64(-1))
	require.Error(t, list.Validate())
	list.Metadata.RemainingItemCount = nil
	list.Items[0].Status = nil
	require.ErrorContains(t, list.Validate(), "items[0].status")
	list.Items = nil
	require.ErrorContains(t, list.Validate(), "items")
	list = NewConnectionToolSetBindingList(nil, "")
	list.Kind = "ToolSetList"
	require.Error(t, list.Validate())
	list = NewConnectionToolSetBindingList(nil, "")
	list.APIVersion = "authproxy.net/v2"
	require.Error(t, list.Validate())
}
