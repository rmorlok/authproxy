package api

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// toolDiscoverySchemaFixture describes a standalone tool without exposing its
// implementation. False hints retain their provenance, and readiness does not
// imply that the connection-health observation is healthy.
func toolDiscoverySchemaFixture(t *testing.T) map[string]any {
	t.Helper()
	return toolResultSchemaDocument(t, `{
		"apiVersion":"authproxy.net/v1alpha1","kind":"ToolDescriptor",
		"toolId":"tol_01example0000001","revision":7,"name":"read-records","description":"Read records.",
		"connection":{"id":"cxn_01example0000001","name":"Production"},
		"readiness":{"state":"ready"},"connectionHealth":{"state":"unhealthy"},
		"hints":{"readOnly":{"value":false,"provenance":"administrator"},"idempotent":{"value":true,"provenance":"provider"}},
		"schema":"none"
	}`)
}

// TestToolDescriptorSchemaDetails checks each requested detail level while
// leaving embedded JSON Schema syntax and provider-specific vocabulary opaque.
func TestToolDescriptorSchemaDetails(t *testing.T) {
	compiled := compileRefSchema(t, "./schema.json#/$defs/ToolDescriptor")
	for _, detail := range []string{"none", "compact", "full"} {
		t.Run(detail, func(t *testing.T) {
			document := toolDiscoverySchemaFixture(t)
			document["schema"] = detail
			if detail != "none" {
				document["inputSchema"] = map[string]any{"type": "object", "unknown_provider_keyword": []any{nil, true, "literal"}}
				document["outputSchema"] = false
			}
			if detail == "full" {
				document["usage"] = "Use the cursor to fetch another page."
				document["examples"] = []any{map[string]any{}, map[string]any{"cursor": "next", "limit": 5}}
			}
			require.NoError(t, compiled.Validate(document))
		})
	}
}

// TestToolDescriptorSchemaFullRoundTrip verifies that the descriptor schema and
// DTO preserve full opaque schemas/examples and explicit false hint values.
func TestToolDescriptorSchemaFullRoundTrip(t *testing.T) {
	compiled := compileRefSchema(t, "./schema.json#/$defs/ToolDescriptor")
	document := toolDiscoverySchemaFixture(t)
	document["schema"] = "full"
	document["inputSchema"] = toolResultSchemaDocument(t, `{"$schema":"provider-dialect","type":"object","x-provider":{"large":9007199254740993,"decimal":0.12345678901234567890123456789}}`)
	document["outputSchema"] = true
	document["usage"] = "Read one page.\nThen pass the returned cursor."
	document["examples"] = []any{toolResultSchemaDocument(t, `{"recordId":9007199254740993,"options":{"cursor":null}}`)}
	document["toolSet"] = map[string]any{"id": "tls_01example0000001", "name": "Records", "generation": 3}
	document["catalogRevision"] = 2
	require.NoError(t, compiled.Validate(document))
	encoded, err := json.Marshal(document)
	require.NoError(t, err)
	var descriptor ToolDescriptor
	require.NoError(t, json.Unmarshal(encoded, &descriptor))
	asYAML, err := yaml.Marshal(descriptor)
	require.NoError(t, err)
	var fromYAML ToolDescriptor
	require.NoError(t, yaml.Unmarshal(asYAML, &fromYAML))
	for _, value := range []*ToolDescriptor{&descriptor, &fromYAML} {
		require.NoError(t, value.Validate(nil))
		roundTrip, err := json.Marshal(value)
		require.NoError(t, err)
		require.JSONEq(t, string(encoded), string(roundTrip))
		require.Contains(t, string(roundTrip), "9007199254740993")
		require.Contains(t, string(roundTrip), "0.12345678901234567890123456789")
		require.NoError(t, compiled.Validate(toolResultSchemaDocument(t, string(roundTrip))))
	}
}

// TestToolDescriptorSchemaBoundaries rejects incomplete provenance, mismatched
// detail levels, and implementation/configuration fields in the projection.
func TestToolDescriptorSchemaBoundaries(t *testing.T) {
	compiled := compileRefSchema(t, "./schema.json#/$defs/ToolDescriptor")
	for _, test := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"missing kind", func(v map[string]any) { delete(v, "kind") }},
		{"resource kind", func(v map[string]any) { v["kind"] = "Tool" }},
		{"wrong tool ID", func(v map[string]any) { v["toolId"] = "tls_01example0000001" }},
		{"zero revision", func(v map[string]any) { v["revision"] = 0 }},
		{"blank name", func(v map[string]any) { v["name"] = " \t" }},
		{"blank description", func(v map[string]any) { v["description"] = "" }},
		{"missing connection", func(v map[string]any) { delete(v, "connection") }},
		{"connection configuration", func(v map[string]any) { v["connection"].(map[string]any)["configuration"] = map[string]any{} }},
		{"catalog without toolset", func(v map[string]any) { v["catalogRevision"] = 1 }},
		{"toolset without generation", func(v map[string]any) { v["toolSet"] = map[string]any{"id": "tls_01example0000001", "name": "Records"} }},
		{"zero toolset generation", func(v map[string]any) {
			v["toolSet"] = map[string]any{"id": "tls_01example0000001", "name": "Records", "generation": 0}
		}},
		{"zero catalog revision", func(v map[string]any) {
			v["toolSet"] = map[string]any{"id": "tls_01example0000001", "name": "Records", "generation": 3}
			v["catalogRevision"] = 0
		}},
		{"missing readiness", func(v map[string]any) { delete(v, "readiness") }},
		{"invalid readiness", func(v map[string]any) { v["readiness"] = map[string]any{"state": "healthy"} }},
		{"blank readiness reason", func(v map[string]any) { v["readiness"] = map[string]any{"state": "unavailable", "reason": " "} }},
		{"invalid connection health", func(v map[string]any) { v["connectionHealth"] = map[string]any{"state": "unknown"} }},
		{"hint without provenance", func(v map[string]any) { v["hints"] = map[string]any{"readOnly": map[string]any{"value": false}} }},
		{"hint without value", func(v map[string]any) {
			v["hints"] = map[string]any{"readOnly": map[string]any{"provenance": "provider"}}
		}},
		{"unknown hint provenance", func(v map[string]any) {
			v["hints"] = map[string]any{"readOnly": map[string]any{"value": true, "provenance": "inferred"}}
		}},
		{"missing detail", func(v map[string]any) { delete(v, "schema") }},
		{"empty detail", func(v map[string]any) { v["schema"] = "" }},
		{"input with none", func(v map[string]any) { v["inputSchema"] = map[string]any{} }},
		{"output with none", func(v map[string]any) { v["outputSchema"] = true }},
		{"compact without input", func(v map[string]any) { v["schema"] = "compact" }},
		{"full without input", func(v map[string]any) { v["schema"] = "full" }},
		{"boolean input schema", func(v map[string]any) { v["schema"] = "full"; v["inputSchema"] = true }},
		{"null output schema", func(v map[string]any) {
			v["schema"] = "full"
			v["inputSchema"] = map[string]any{}
			v["outputSchema"] = nil
		}},
		{"compact usage", func(v map[string]any) {
			v["schema"] = "compact"
			v["inputSchema"] = map[string]any{}
			v["usage"] = "Read a page."
		}},
		{"none examples", func(v map[string]any) { v["examples"] = []any{} }},
		{"nonobject example", func(v map[string]any) {
			v["schema"] = "full"
			v["inputSchema"] = map[string]any{}
			v["examples"] = []any{nil}
		}},
		{"object examples", func(v map[string]any) {
			v["schema"] = "full"
			v["inputSchema"] = map[string]any{}
			v["examples"] = map[string]any{}
		}},
		{"executor leakage", func(v map[string]any) { v["javascript"] = "return {};" }},
		{"proxy plan leakage", func(v map[string]any) { v["proxyHttp"] = map[string]any{"method": "GET", "url": "https://example.com"} }},
		{"configuration leakage", func(v map[string]any) { v["configuration"] = map[string]any{"secret": "value"} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			document := toolDiscoverySchemaFixture(t)
			test.change(document)
			require.Error(t, compiled.Validate(document))
		})
	}
}

// TestToolDiscoverySchemasIndependentObservations allows every readiness and
// health combination; the two observations describe different state.
func TestToolDiscoverySchemasIndependentObservations(t *testing.T) {
	compiled := compileRefSchema(t, "./schema.json#/$defs/ToolDescriptor")
	for _, readiness := range []string{"ready", "unavailable", "unknown"} {
		for _, health := range []string{"", "healthy", "unhealthy"} {
			document := toolDiscoverySchemaFixture(t)
			document["readiness"] = map[string]any{"state": readiness}
			if health == "" {
				delete(document, "connectionHealth")
			} else {
				document["connectionHealth"] = map[string]any{"state": health}
			}
			require.NoError(t, compiled.Validate(document), "%s/%s", readiness, health)
		}
	}
}

// TestToolDiscoverySchemaLists uses the common paginated list envelope while
// preserving canProxy:false and keeping connection summaries intentionally small.
func TestToolDiscoverySchemaLists(t *testing.T) {
	connection := map[string]any{"apiVersion": "authproxy.net/v1alpha1", "kind": "AgentConnection", "id": "cxn_01example0000001", "name": "Production", "canProxy": false}
	for _, test := range []struct {
		name, kind string
		item       map[string]any
	}{
		{"tools", "ToolDescriptorList", toolDiscoverySchemaFixture(t)},
		{"connections", "AgentConnectionList", connection},
	} {
		t.Run(test.name, func(t *testing.T) {
			compiled := compileRefSchema(t, "./schema.json#/$defs/"+test.kind)
			document := map[string]any{"apiVersion": "authproxy.net/v1alpha1", "kind": test.kind, "metadata": map[string]any{"continue": "next-page"}, "items": []any{test.item}}
			require.NoError(t, compiled.Validate(document))
			document["items"] = []any{}
			require.NoError(t, compiled.Validate(document))
			document["items"] = nil
			require.Error(t, compiled.Validate(document))
			document["items"] = []any{}
			document["continue"] = "legacy-position"
			require.Error(t, compiled.Validate(document))
		})
	}
	compiled := compileRefSchema(t, "./schema.json#/$defs/AgentConnectionSummary")
	delete(connection, "canProxy")
	require.Error(t, compiled.Validate(connection), "false must be explicit rather than omitted")
	connection["canProxy"] = false
	connection["configuration"] = map[string]any{}
	require.Error(t, compiled.Validate(connection), "connection configuration is not discoverable")
}

// TestToolSearchQuerySchema documents supported filters and bounded result size.
// Connection restrictions use labels, leaving no alternate connectionId filter.
func TestToolSearchQuerySchema(t *testing.T) {
	compiled := compileRefSchema(t, "./schema.json#/$defs/ToolSearchQuery")
	for _, input := range []string{
		`{}`,
		`{"schema":""}`,
		`{"schema":"none","limit":1}`,
		`{"schema":"compact","limit":100,"includeUnavailable":true}`,
		`{"query":"read records","labelSelector":"apxy/cxn/-/id=cxn_01example0000001","toolSetId":"tls_01example0000001","schema":"full","limit":5,"continue":"opaque"}`,
	} {
		require.NoError(t, compiled.Validate(toolResultSchemaDocument(t, input)), input)
	}
	for _, input := range []string{
		`{"limit":0}`, `{"limit":101}`, `{"limit":-1}`, `{"limit":1.5}`, `{"limit":null}`,
		`{"schema":"summary"}`, `{"toolSetId":"cxn_01example0000001"}`,
		`{"connectionId":"cxn_01example0000001"}`, `{"includeUnavailable":"true"}`,
	} {
		require.Error(t, compiled.Validate(toolResultSchemaDocument(t, input)), input)
	}
}
