package api

import (
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	connectionschema "github.com/rmorlok/authproxy/internal/schema/resources/connection"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// toolDescriptorForTest supplies a standalone public identity with unknown
// readiness and no schemas, matching the default discovery detail level.
func toolDescriptorForTest() *ToolDescriptor {
	descriptor := NewToolDescriptor()
	descriptor.ToolID, descriptor.Revision = "tol_01example0000001", 7
	descriptor.Name, descriptor.Description = "read-events", "Read calendar events."
	descriptor.Connection = ToolConnectionIdentity{ID: "cxn_01example0000001", Name: "calendar"}
	return descriptor
}

// TestToolDescriptorValidation verifies public identity, provenance, and readiness
// independently of probe health or the permissions already checked by producers.
func TestToolDescriptorValidation(t *testing.T) {
	require.Error(t, (*ToolDescriptor)(nil).Validate(nil))
	require.Error(t, NewToolDescriptor().Validate(nil))

	for _, test := range []struct {
		name   string
		change func(*ToolDescriptor)
		field  string
	}{
		{"default unknown", func(d *ToolDescriptor) {}, ""},
		{"ready with unhealthy probes", func(d *ToolDescriptor) {
			d.Readiness.State = ToolReadinessStateReady
			d.ConnectionHealth = &connectionschema.ConnectionHealthStatus{State: connectionschema.ConnectionHealthStateUnhealthy}
		}, ""},
		{"unavailable with future reason", func(d *ToolDescriptor) {
			d.Readiness = ToolReadiness{State: ToolReadinessStateUnavailable, Reason: "FUTURE_RECOVERY", Message: "Additional setup is needed."}
		}, ""},
		{"generated catalog", func(d *ToolDescriptor) {
			d.ToolSet = &ToolSetIdentity{ID: "tls_01example0000001", Name: "calendar-tools", Generation: 2}
			d.CatalogRevision = util.ToPtr(uint64(9))
		}, ""},
		{"wrong version", func(d *ToolDescriptor) { d.APIVersion = "authproxy.net/v2" }, "apiVersion"},
		{"wrong kind", func(d *ToolDescriptor) { d.Kind = "Tool" }, "kind"},
		{"wrong tool ID", func(d *ToolDescriptor) { d.ToolID = d.Connection.ID }, "toolId"},
		{"zero revision", func(d *ToolDescriptor) { d.Revision = 0 }, "revision"},
		{"blank name", func(d *ToolDescriptor) { d.Name = "\t" }, "name"},
		{"blank description", func(d *ToolDescriptor) { d.Description = " " }, "description"},
		{"wrong connection ID", func(d *ToolDescriptor) { d.Connection.ID = d.ToolID }, "connection.id"},
		{"blank connection name", func(d *ToolDescriptor) { d.Connection.Name = " " }, "connection.name"},
		{"invalid source identity", func(d *ToolDescriptor) { d.ToolSet = &ToolSetIdentity{} }, "toolSet.id"},
		{"zero generation", func(d *ToolDescriptor) {
			d.ToolSet = &ToolSetIdentity{ID: "tls_01example0000001", Name: "calendar-tools"}
		}, "toolSet.generation"},
		{"catalog without source", func(d *ToolDescriptor) { d.CatalogRevision = util.ToPtr(uint64(9)) }, "toolSet"},
		{"zero catalog", func(d *ToolDescriptor) { d.CatalogRevision = util.ToPtr(uint64(0)) }, "catalogRevision"},
		{"unknown readiness", func(d *ToolDescriptor) { d.Readiness.State = "healthy" }, "readiness.state"},
		{"blank reason", func(d *ToolDescriptor) { d.Readiness.Reason = " " }, "readiness.reason"},
		{"unknown health", func(d *ToolDescriptor) {
			d.ConnectionHealth = &connectionschema.ConnectionHealthStatus{State: "unknown"}
		}, "connectionHealth.state"},
		{"missing provenance", func(d *ToolDescriptor) { d.Hints = &ToolDescriptorHints{ReadOnly: &ToolHint{Value: true}} }, "hints.readOnly.provenance"},
		{"unknown provenance", func(d *ToolDescriptor) {
			d.Hints = &ToolDescriptorHints{Destructive: &ToolHint{Provenance: "inferred"}}
		}, "hints.destructive.provenance"},
	} {
		t.Run(test.name, func(t *testing.T) {
			descriptor := toolDescriptorForTest()
			test.change(descriptor)
			err := descriptor.Validate(&common.ValidationContext{Path: "response"})
			if test.field == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "response."+test.field)
			}
		})
	}
}

// TestToolDescriptorSchemaDetail keeps compact schemas executable and full-only
// guidance separate, without asking API validation to compile provider schemas.
func TestToolDescriptorSchemaDetail(t *testing.T) {
	for _, detail := range []ToolSchemaDetail{ToolSchemaDetailCompact, ToolSchemaDetailFull} {
		descriptor := toolDescriptorForTest()
		descriptor.Schema = detail
		require.ErrorContains(t, descriptor.Validate(nil), "inputSchema")
		descriptor.InputSchema = common.RawJSON(`{"type":"object","$ref":"#/$defs/input"}`)
		for _, output := range []common.RawJSON{nil, common.RawJSON(`{}`), common.RawJSON(`false`)} {
			descriptor.OutputSchema = output
			require.NoError(t, descriptor.Validate(nil), "schema compilation is a producer responsibility")
		}
	}

	for _, test := range []struct {
		change func(*ToolDescriptor)
		field  string
	}{
		{func(d *ToolDescriptor) { d.Schema = "" }, "schema"},
		{func(d *ToolDescriptor) { d.Schema = "preview" }, "schema"},
		{func(d *ToolDescriptor) { d.Schema = ToolSchemaDetailNone }, "inputSchema"},
		{func(d *ToolDescriptor) { d.InputSchema = common.RawJSON(`false`) }, "inputSchema"},
		{func(d *ToolDescriptor) { d.InputSchema = common.RawJSON(`null`) }, "inputSchema"},
		{func(d *ToolDescriptor) { d.OutputSchema = common.RawJSON(`[]`) }, "outputSchema"},
		{func(d *ToolDescriptor) { d.OutputSchema = common.RawJSON(`null`) }, "outputSchema"},
		{func(d *ToolDescriptor) { d.OutputSchema = common.RawJSON(`{} {}`) }, "outputSchema"},
		{func(d *ToolDescriptor) { d.Schema, d.Usage = ToolSchemaDetailCompact, "Usage guidance" }, "usage"},
		{func(d *ToolDescriptor) { d.Schema, d.Examples = ToolSchemaDetailCompact, common.RawJSON(`[{}]`) }, "examples"},
		{func(d *ToolDescriptor) { d.Examples = common.RawJSON(`{}`) }, "examples"},
		{func(d *ToolDescriptor) { d.Examples = common.RawJSON(`[null]`) }, "examples"},
		{func(d *ToolDescriptor) { d.Examples = common.RawJSON(`[7]`) }, "examples"},
	} {
		descriptor := toolDescriptorForTest()
		descriptor.Schema, descriptor.InputSchema = ToolSchemaDetailFull, common.RawJSON(`{"type":"object"}`)
		test.change(descriptor)
		require.ErrorContains(t, descriptor.Validate(nil), test.field)
	}
}

// TestToolDiscoveryRoundTrip preserves full usage data and mixed hint origins
// while strict decoding excludes management-only fields from public projections.
func TestToolDiscoveryRoundTrip(t *testing.T) {
	for _, format := range []struct {
		name   string
		encode func(any) ([]byte, error)
		decode func([]byte, any) error
	}{
		{"JSON", json.Marshal, util.DecodeJSONStrict},
		{"YAML", yaml.Marshal, util.DecodeYAMLStrict},
	} {
		t.Run(format.name, func(t *testing.T) {
			descriptor := toolDescriptorForTest()
			descriptor.Schema, descriptor.InputSchema = ToolSchemaDetailFull, common.RawJSON(`{"type":"object"}`)
			descriptor.OutputSchema, descriptor.Usage = common.RawJSON(`false`), "Call once for each calendar."
			descriptor.Examples = common.RawJSON(`[{"n":9007199254740993,"nested":{"empty":null}}]`)
			descriptor.Hints = &ToolDescriptorHints{
				ReadOnly:    &ToolHint{Value: true, Provenance: ToolHintProvenanceAdministrator},
				Destructive: &ToolHint{Value: false, Provenance: ToolHintProvenanceProvider},
			}
			data, err := format.encode(descriptor)
			require.NoError(t, err)
			var decoded ToolDescriptor
			require.NoError(t, format.decode(data, &decoded))
			require.Equal(t, descriptor, &decoded)
			require.NoError(t, decoded.Validate(nil))
			require.NotNil(t, decoded.Hints.Destructive)
			require.False(t, decoded.Hints.Destructive.Value)
			require.Nil(t, decoded.Hints.Idempotent)

			for _, input := range []string{
				`{"javascript":"privileged source"}`, `{"proxyHttp":{}}`, `{"verbs":["tool:read"]}`,
				`{"annotations":{"private":"value"}}`, `{"connection":{"configuration":{"tenant":"private"}}}`,
			} {
				var unsafe ToolDescriptor
				require.Error(t, format.decode([]byte(input), &unsafe))
			}

			summary := NewAgentConnectionSummary(descriptor.Connection, false)
			data, err = format.encode(summary)
			require.NoError(t, err)
			var decodedSummary AgentConnectionSummary
			require.NoError(t, format.decode(data, &decodedSummary))
			require.Equal(t, summary, decodedSummary)
			require.NoError(t, decodedSummary.Validate(nil))
			require.False(t, decodedSummary.CanProxy)
			var unsafeSummary AgentConnectionSummary
			require.Error(t, format.decode([]byte(`{"configuration":{}}`), &unsafeSummary))
		})
	}
}

// TestToolDiscoveryLists validates page metadata and each public item without
// granting proxy access or interpreting continuation tokens as resource identity.
func TestToolDiscoveryLists(t *testing.T) {
	descriptors := NewToolDescriptorList(nil, "next+/=")
	require.NotNil(t, descriptors.Items)
	require.Empty(t, descriptors.Items)
	require.Equal(t, "next+/=", descriptors.Metadata.Continue)
	require.NoError(t, descriptors.Validate())
	descriptors.Items = []ToolDescriptor{*toolDescriptorForTest()}
	descriptors.Metadata.RemainingItemCount = util.ToPtr(int64(4))
	require.NoError(t, descriptors.Validate())
	descriptors.Metadata.RemainingItemCount = util.ToPtr(int64(-1))
	require.Error(t, descriptors.Validate())
	descriptors.Metadata.RemainingItemCount = nil
	descriptors.Items[0].Revision = 0
	require.ErrorContains(t, descriptors.Validate(), "items[0].revision")
	descriptors.Items = nil
	require.ErrorContains(t, descriptors.Validate(), "items")
	descriptors = NewToolDescriptorList(nil, "")
	descriptors.Kind = "ToolList"
	require.Error(t, descriptors.Validate())

	identity := toolDescriptorForTest().Connection
	for _, canProxy := range []bool{false, true} {
		summary := NewAgentConnectionSummary(identity, canProxy)
		require.NoError(t, summary.Validate(nil))
	}
	connections := NewAgentConnectionList(nil, "")
	require.NotNil(t, connections.Items)
	require.NoError(t, connections.Validate())
	connections.Items = []AgentConnectionSummary{NewAgentConnectionSummary(identity, false)}
	require.NoError(t, connections.Validate())
	connections.Items[0].ID = "tls_01example0000001"
	require.ErrorContains(t, connections.Validate(), "items[0].id")
	connections.Items = nil
	require.ErrorContains(t, connections.Validate(), "items")
	connections = NewAgentConnectionList(nil, "")
	connections.APIVersion = meta.APIVersion("authproxy.net/v2")
	require.Error(t, connections.Validate())
	require.Error(t, (*AgentConnectionSummary)(nil).Validate(nil))
	require.Error(t, (*ToolDescriptorList)(nil).Validate())
	require.Error(t, (*AgentConnectionList)(nil).Validate())
}
