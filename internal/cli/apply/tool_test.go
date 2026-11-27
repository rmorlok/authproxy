package apply

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	toolschema "github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/stretchr/testify/require"
)

// toolApplySpec supplies a complete authored definition with the caller's
// connection reference, allowing each scenario to focus on reference handling.
func toolApplySpec(reference string) string {
	return fmt.Sprintf(`{"connectionRef":%s,"description":"Read an account","verbs":["read"],"inputSchema":{"type":"object"},"proxyHttp":{"method":"GET","url":"https://example.com/account"}}`, reference)
}

// newToolApplyServer extends the shared in-memory server with Tool collection
// reads. Both existing resources support the ordinary typed PATCH path; these
// tests exercise CLI planning rather than the server's revision publication.
func newToolApplyServer(t *testing.T) (*batchServer, *Client, string, string) {
	t.Helper()
	server := &batchServer{t: t, objects: map[string]map[string]any{
		"namespaces/root": {
			"apiVersion": string(meta.APIVersionV1Alpha1), "kind": "Namespace",
			"metadata": map[string]any{"id": "root", "name": "root"}, "spec": map[string]any{},
		},
	}}
	connectionID := apid.New(apid.PrefixConnection).String()
	toolID := apid.New(apid.PrefixTool).String()
	connection := reconcileLive(t, "Connection", connectionID, fmt.Sprintf(
		`{"connectorRef":{"apiVersion":"authproxy.net/v1alpha1","kind":"Connector","id":%q,"generation":1}}`,
		apid.New(apid.PrefixConnector).String()), nil)
	object, err := plainObject(connection.Resource)
	require.NoError(t, err)
	liveMeta(object)["name"] = "account"
	object["status"] = map[string]any{
		"lifecycle":     map[string]any{"state": "configured"},
		"health":        map[string]any{"state": "healthy"},
		"configuration": map[string]any{"configured": true, "schema": map[string]any{"type": "object"}},
	}
	server.objects["connections/"+connectionID] = object

	doc := clientDoc(t, "Tool", "  name: example\n  namespace: root", toolApplySpec(fmt.Sprintf(
		`{"apiVersion":"authproxy.net/v1alpha1","kind":"Connection","id":%q}`, connectionID)))
	object, err = plainObject(doc.Object)
	require.NoError(t, err)
	liveMeta(object)["id"] = toolID
	object["status"] = map[string]any{"revision": 1}
	server.objects["tools/"+toolID] = object

	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/tools" {
			server.handle(w, r)
			return
		}
		var items []string
		for path, item := range server.objects {
			if !strings.HasPrefix(path, "tools/") {
				continue
			}
			metadata := liveMeta(item)
			if metadata["namespace"] != r.URL.Query().Get("namespace") || metadata["name"] != r.URL.Query().Get("name") {
				continue
			}
			data, err := json.Marshal(item)
			require.NoError(t, err)
			items = append(items, string(data))
		}
		fmt.Fprint(w, listJSON("Tool", items, ""))
	}, false)
	return server, client, toolID, connectionID
}

// TestToolApplyNamedConnectionAndHistory verifies that canonical IDs are used
// for reconciliation and adoption without changing the caller's manifest.
func TestToolApplyNamedConnectionAndHistory(t *testing.T) {
	server, client, toolID, connectionID := newToolApplyServer(t)
	ctx := context.Background()
	doc := clientDoc(t, "Tool", "  name: example\n  namespace: root", toolApplySpec(
		`{"apiVersion":"authproxy.net/v1alpha1","kind":"Connection","name":"account","namespace":"root"}`))
	original, err := json.Marshal(doc.Object)
	require.NoError(t, err)
	originalRef := doc.Resource.(*toolschema.Tool).Spec.ConnectionRef
	canonical := map[string]any{"apiVersion": "authproxy.net/v1alpha1", "kind": "Connection", "id": connectionID}

	// Resolution normalizes both retained JSON and the typed resource on a copy.
	targets, err := client.ResolveBatch(ctx, []Document{doc})
	require.NoError(t, err)
	require.Equal(t, canonical, asSpec(targets[0].Document.Object)["connectionRef"])
	require.Equal(t, connectionID, targets[0].Document.Resource.(*toolschema.Tool).Spec.ConnectionRef.ID)
	unchanged, err := json.Marshal(doc.Object)
	require.NoError(t, err)
	require.Equal(t, string(original), string(unchanged))
	require.Equal(t, originalRef, doc.Resource.(*toolschema.Tool).Spec.ConnectionRef)
	require.Empty(t, server.writes)

	// Explicit history adoption uses the same canonical form as normal apply.
	historyTargets, err := client.HistoryTargets(ctx, []Document{doc})
	require.NoError(t, err)
	_, err = client.SetLastApplied(ctx, historyTargets, []Document{doc}, true)
	require.NoError(t, err)
	historyTargets, err = client.HistoryTargets(ctx, []Document{doc})
	require.NoError(t, err)
	desired, err := historyTargets[0].LastApplied()
	require.NoError(t, err)
	require.Equal(t, canonical, asSpec(desired)["connectionRef"])
	require.Equal(t, toolID, liveMeta(desired)["id"])

	// The named manifest now reconciles without publishing another update.
	batch, err := client.Prepare(ctx, []Document{doc}, ReconcileOptions{Overwrite: true})
	require.NoError(t, err)
	results, err := batch.Execute(ctx)
	require.NoError(t, err)
	require.Equal(t, "unchanged", results[0].Status)
	require.Equal(t, []string{"PATCH tools/example"}, server.writes)
}

// TestToolApplyIDOnlyUsesLiveNamespace covers partial manifests that identify
// the Tool by ID and rely on the existing resource for its namespace boundary.
func TestToolApplyIDOnlyUsesLiveNamespace(t *testing.T) {
	server, client, toolID, connectionID := newToolApplyServer(t)
	liveMeta(server.objects["tools/"+toolID])["namespace"] = "root.team"
	liveMeta(server.objects["connections/"+connectionID])["namespace"] = "root.team"
	doc := clientDoc(t, "Tool", "  id: "+toolID, toolApplySpec(
		`{"apiVersion":"authproxy.net/v1alpha1","kind":"Connection","name":"account","namespace":"root.team"}`))

	// No manifest namespace is required when the live Tool establishes it.
	targets, err := client.ResolveBatch(context.Background(), []Document{doc})
	require.NoError(t, err)
	require.Equal(t, "root.team", targets[0].Current.Metadata.Namespace)
	require.Equal(t, connectionID, targets[0].Document.Resource.(*toolschema.Tool).Spec.ConnectionRef.ID)
	require.Empty(t, doc.Metadata.Namespace)
	require.Empty(t, server.writes)
}

// TestToolApplyInvalidConnectionPreventsBatchWrites ensures reference errors
// are discovered before an otherwise valid earlier metadata update is applied.
func TestToolApplyInvalidConnectionPreventsBatchWrites(t *testing.T) {
	for _, tc := range []struct {
		name, reference, connectionNamespace, message string
	}{
		{"missing", `"name":"missing","namespace":"root"`, "root", "not found"},
		{"cross namespace ID", `"id":%q`, "root.other", "Tool namespace"},
		{"cross namespace name", `"name":"account","namespace":"root.other"`, "root.other", "invalid Tool connection reference"},
		{"mismatched ID and name", `"id":%q,"name":"wrong","namespace":"root"`, "root", "does not match"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, client, _, connectionID := newToolApplyServer(t)
			liveMeta(server.objects["connections/"+connectionID])["namespace"] = tc.connectionNamespace
			reference := tc.reference
			if strings.Contains(reference, "%q") {
				reference = fmt.Sprintf(reference, connectionID)
			}
			docs := []Document{
				clientDoc(t, "Connection", "  id: "+connectionID+"\n  labels: {updated: yes}", `{}`),
				clientDoc(t, "Tool", "  name: example\n  namespace: root", toolApplySpec(
					`{"apiVersion":"authproxy.net/v1alpha1","kind":"Connection",`+reference+`}`)),
			}

			// Prevalidation is atomic across all selected manifests.
			_, err := client.Prepare(context.Background(), docs, ReconcileOptions{Overwrite: true})
			require.ErrorContains(t, err, tc.message)
			require.Empty(t, server.writes)
		})
	}
}

// TestToolApplyConnectionDependency verifies that a named reference still
// orders the corresponding Connection update before its Tool after resolution.
func TestToolApplyConnectionDependency(t *testing.T) {
	server, client, _, _ := newToolApplyServer(t)
	docs := []Document{
		clientDoc(t, "Tool", "  name: example\n  namespace: root\n  labels: {updated: yes}", toolApplySpec(
			`{"apiVersion":"authproxy.net/v1alpha1","kind":"Connection","name":"account","namespace":"root"}`)),
		clientDoc(t, "Connection", "  name: account\n  namespace: root\n  labels: {updated: yes}", `{}`),
	}

	// The dependent Tool appears first in input but must execute second.
	batch, err := client.Prepare(context.Background(), docs, ReconcileOptions{Overwrite: true})
	require.NoError(t, err)
	require.Empty(t, server.writes)
	_, err = batch.Execute(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"PATCH connections/account", "PATCH tools/example"}, server.writes)
}

// TestToolApplyRejectsGeneratedNoOp protects controller ownership even when
// the requested spec and the saved apply history would otherwise be unchanged.
func TestToolApplyRejectsGeneratedNoOp(t *testing.T) {
	server, client, toolID, _ := newToolApplyServer(t)
	doc := clientDoc(t, "Tool", "  id: "+toolID+"\n  name: example\n  namespace: root", `{}`)
	live, err := client.Resolve(context.Background(), doc)
	require.NoError(t, err)
	live = withHistory(t, live, doc)
	plan, err := Reconcile(Target{Document: doc, Current: live}, ReconcileOptions{Overwrite: true})
	require.NoError(t, err)
	require.Equal(t, OperationUnchanged, plan.Operation)

	// Identical authored state does not authorize adopting a ToolSet's Tool.
	toolSetID := apid.New(apid.PrefixToolSet).String()
	tool := live.Resource.(*toolschema.Tool)
	tool.Status.ManagedBy = &toolschema.ToolManagedBy{
		ToolSetRef: meta.ObjectReference{APIVersion: meta.APIVersionV1Alpha1, Kind: "ToolSet", ID: toolSetID, Generation: 1},
		SourceKey:  "read-account",
	}
	server.objects["tools/"+toolID], err = plainObject(tool)
	require.NoError(t, err)
	_, err = client.Prepare(context.Background(), []Document{doc}, ReconcileOptions{Overwrite: true})
	require.ErrorContains(t, err, "managed by ToolSet "+toolSetID)
	historyTargets, err := client.HistoryTargets(context.Background(), []Document{doc})
	require.NoError(t, err)
	_, err = client.SetLastApplied(context.Background(), historyTargets, []Document{doc}, true)
	require.ErrorContains(t, err, "managed by ToolSet "+toolSetID)
	require.Empty(t, server.writes)
}

// TestToolApplyPruneUnsupported keeps Tool deletion outside the existing
// guarded prune contract even though standalone Tool writes are supported.
func TestToolApplyPruneUnsupported(t *testing.T) {
	options := PruneOptions{Namespace: "root", All: true, Allowlist: []string{"authproxy.net/v1alpha1/Tool"}}
	require.ErrorContains(t, options.Validate(), "prune supports only Actor, Key and RateLimit")
}

// TestToolLoadUnknownFieldsPreservesJSON verifies that validation-mode filtering
// only removes unknown contract fields, never user-defined schema or body keys.
func TestToolLoadUnknownFieldsPreservesJSON(t *testing.T) {
	input := manifestText("Tool", "  name: example\n  namespace: root", `{
  connectionRef: {apiVersion: authproxy.net/v1alpha1, kind: Connection, name: account, namespace: root},
  description: Read an account,
  verbs: [read],
  inputSchema: {type: object, properties: {unknown: {type: string}}, x-custom: {arbitrary: retained}},
  proxyHttp: {method: POST, url: https://example.com/account, bodyJson: {unknown: {nested: [one, two]}}, unknown: dropped},
  unknown: dropped
}`)
	for _, mode := range []Validation{ValidationWarn, ValidationIgnore} {
		t.Run(string(mode), func(t *testing.T) {
			var warnings []string
			docs, err := Load(context.Background(), Options{
				Filenames: []string{"-"}, Stdin: strings.NewReader(input), Validation: mode,
				Warn: func(message string) { warnings = append(warnings, message) },
			})
			require.NoError(t, err)
			spec := asSpec(docs[0].Object)
			require.NotContains(t, spec, "unknown")
			require.NotContains(t, spec["proxyHttp"], "unknown")
			tool := docs[0].Resource.(*toolschema.Tool)
			require.JSONEq(t, `{"type":"object","properties":{"unknown":{"type":"string"}},"x-custom":{"arbitrary":"retained"}}`, string(tool.Spec.InputSchema))
			require.JSONEq(t, `{"unknown":{"nested":["one","two"]}}`, string(tool.Spec.ProxyHTTP.BodyJSON))
			require.Equal(t, mode == ValidationWarn, len(warnings) > 0)
		})
	}
}

// TestToolLoadRejectsServerOwnedFields ensures neither generation nor status
// can become authored state, including when unknown fields would be ignored.
func TestToolLoadRejectsServerOwnedFields(t *testing.T) {
	for _, input := range []string{
		manifestText("Tool", "  name: example\n  namespace: root\n  generation: 0", `{}`),
		manifestText("Tool", "  name: example\n  namespace: root\n  generation: 1", `{}`),
		manifestText("Tool", "  name: example\n  namespace: root", `{}`) + "status: null\n",
		manifestText("Tool", "  name: example\n  namespace: root", `{}`) + "status: {revision: 1}\n",
	} {
		for _, mode := range []Validation{ValidationStrict, ValidationIgnore} {
			docs, err := Load(context.Background(), Options{Filenames: []string{"-"}, Stdin: strings.NewReader(input), Validation: mode})
			require.Error(t, err)
			require.Nil(t, docs)
		}
	}
}
