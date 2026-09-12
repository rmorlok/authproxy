package apply

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/registry"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
)

func reconcileLive(t *testing.T, kind, id, spec string, annotations map[string]string) *LiveResource {
	t.Helper()
	c := &Client{scheme: registry.NewResourceScheme()}
	object := map[string]any{}
	require.NoError(t, json.Unmarshal([]byte(liveJSON(kind, id, "example", "root", spec)), &object))
	if annotations != nil {
		object["metadata"].(map[string]any)["annotations"] = annotations
	}
	data, err := json.Marshal(object)
	require.NoError(t, err)
	live, err := c.decodeLive(meta.Kind(kind), data, false)
	require.NoError(t, err)
	return live
}
func withHistory(t *testing.T, live *LiveResource, doc Document) *LiveResource {
	t.Helper()
	h, err := newHistory(doc)
	require.NoError(t, err)
	h.Desired["metadata"].(map[string]any)["id"] = live.Metadata.ID
	object, err := plainObject(live.Resource)
	require.NoError(t, err)
	require.NoError(t, attachHistory(object, h))
	data, err := json.Marshal(object)
	require.NoError(t, err)
	result, err := (&Client{scheme: registry.NewResourceScheme()}).decodeLive(live.Kind, data, live.Redacted)
	require.NoError(t, err)
	return result
}
func planObject(t *testing.T, plan *Plan) map[string]any {
	t.Helper()
	var object map[string]any
	require.NoError(t, json.Unmarshal(plan.Patch, &object))
	return object
}
func commitPlan(t *testing.T, plan *Plan) *LiveResource {
	t.Helper()
	descriptor, err := resourceType(plan.Target.Document.Kind)
	require.NoError(t, err)
	patch, err := descriptor.DecodePatchJSON(plan.Patch)
	require.NoError(t, err)
	updated, err := descriptor.ApplyPatch(plan.Target.Current.Resource, patch)
	require.NoError(t, err)
	data, err := json.Marshal(updated)
	require.NoError(t, err)
	live, err := (&Client{scheme: registry.NewResourceScheme()}).decodeLive(plan.Target.Document.Kind, data, false)
	require.NoError(t, err)
	return live
}

func TestReconcileAdoptionMapsAndIdempotence(t *testing.T) {
	live := reconcileLive(t, "Actor", apid.New(apid.PrefixActor).String(), `{"externalId":"subject"}`, map[string]string{"external": "keep"})
	doc := clientDoc(t, "Actor", "  name: example\n  namespace: root\n  labels: {managed: yes}", "{}")
	before, _ := json.Marshal(live.Resource)
	plan, err := Reconcile(Target{doc, live}, ReconcileOptions{Overwrite: true})
	require.NoError(t, err)
	require.Equal(t, OperationUpdate, plan.Operation)
	require.Len(t, plan.Warnings, 1)
	object := planObject(t, plan)
	require.Empty(t, object["spec"])
	require.Equal(t, "keep", object["metadata"].(map[string]any)["annotations"].(map[string]any)["external"])
	after, _ := json.Marshal(live.Resource)
	require.Equal(t, string(before), string(after))
	updated := commitPlan(t, plan)
	again, err := Reconcile(Target{doc, updated}, ReconcileOptions{Overwrite: true})
	require.NoError(t, err)
	require.Equal(t, OperationUnchanged, again.Operation)
	require.Empty(t, again.Patch)
	require.NotContains(t, doc.Object["metadata"], "annotations")
}

func TestReconcileOmittedManagedMapsPreserveUnmanaged(t *testing.T) {
	live := reconcileLive(t, "Actor", apid.New(apid.PrefixActor).String(), `{"externalId":"subject"}`, map[string]string{"managed": "old", "external": "keep"})
	old := clientDoc(t, "Actor", "  name: example\n  namespace: root\n  annotations: {managed: old}", "{}")
	live = withHistory(t, live, old)
	for _, metadata := range []string{"", "\n  annotations: {}", "\n  annotations: null"} {
		doc := clientDoc(t, "Actor", "  name: example\n  namespace: root"+metadata, "{}")
		plan, err := Reconcile(Target{doc, live}, ReconcileOptions{Overwrite: true})
		require.NoError(t, err)
		annotations := planObject(t, plan)["metadata"].(map[string]any)["annotations"].(map[string]any)
		require.NotContains(t, annotations, "managed")
		require.Equal(t, "keep", annotations["external"])
	}
}

func TestReconcileDrift(t *testing.T) {
	old := clientDoc(t, "Actor", "  name: example\n  namespace: root\n  annotations: {managed: original}", "{}")
	live := withHistory(t, reconcileLive(t, "Actor", apid.New(apid.PrefixActor).String(), `{"externalId":"subject"}`, map[string]string{"managed": "sensitive-drift"}), old)
	_, err := Reconcile(Target{old, live}, ReconcileOptions{})
	require.ErrorContains(t, err, "conflict")
	require.NotContains(t, err.Error(), "sensitive-drift")
	plan, err := Reconcile(Target{old, live}, ReconcileOptions{Overwrite: true})
	require.NoError(t, err)
	require.Equal(t, "original", planObject(t, plan)["metadata"].(map[string]any)["annotations"].(map[string]any)["managed"])
	same := clientDoc(t, "Actor", "  name: example\n  namespace: root\n  annotations: {managed: sensitive-drift}", "{}")
	_, err = Reconcile(Target{same, live}, ReconcileOptions{})
	require.NoError(t, err)
}

func TestReconcileNullableAndIllegalRemoval(t *testing.T) {
	keyID := apid.New(apid.PrefixKey).String()
	ref := fmt.Sprintf(`{"encryptionKeyRef":{"apiVersion":"authproxy.net/v1alpha1","kind":"Key","id":%q}}`, keyID)
	old := clientDoc(t, "Namespace", "  name: example\n  namespace: root", ref)
	live := withHistory(t, reconcileLive(t, "Namespace", "root.example", ref, nil), old)
	for _, spec := range []string{"{}", `{"encryptionKeyRef":null}`} {
		doc := clientDoc(t, "Namespace", "  name: example\n  namespace: root", spec)
		plan, err := Reconcile(Target{doc, live}, ReconcileOptions{Overwrite: true})
		require.NoError(t, err)
		patch := planObject(t, plan)["spec"].(map[string]any)
		require.Contains(t, patch, "encryptionKeyRef")
		require.Nil(t, patch["encryptionKeyRef"])
	}
	actorOld := clientDoc(t, "Actor", "  name: example\n  namespace: root", `{"externalId":"subject"}`)
	actorLive := withHistory(t, reconcileLive(t, "Actor", apid.New(apid.PrefixActor).String(), `{"externalId":"subject"}`, nil), actorOld)
	_, err := Reconcile(Target{clientDoc(t, "Actor", "  name: example\n  namespace: root", "{}"), actorLive}, ReconcileOptions{Overwrite: true})
	require.Error(t, err)
}

func TestReconcileConnectorReplacementAndUnchanged(t *testing.T) {
	spec := `{"definition":{"displayName":"Old","description":"unmanaged","auth":{"type":"no-auth"}}}`
	old := clientDoc(t, "Connector", "  name: example\n  namespace: root", `{"definition":{"displayName":"Old","auth":{"type":"no-auth"}}}`)
	live := withHistory(t, reconcileLive(t, "Connector", apid.New(apid.PrefixConnector).String(), spec, nil), old)
	plan, err := Reconcile(Target{old, live}, ReconcileOptions{Overwrite: true})
	require.NoError(t, err)
	require.Equal(t, OperationUnchanged, plan.Operation)
	doc := clientDoc(t, "Connector", "  name: example\n  namespace: root", `{"definition":{"displayName":"New","auth":{"type":"no-auth"}}}`)
	plan, err = Reconcile(Target{doc, live}, ReconcileOptions{Overwrite: true})
	require.NoError(t, err)
	definition := planObject(t, plan)["spec"].(map[string]any)["definition"].(map[string]any)
	require.Equal(t, "New", definition["displayName"])
	require.Equal(t, "unmanaged", definition["description"])
	require.NotContains(t, planObject(t, plan)["spec"], "release")
}

func TestReconcileSecretsAndHistory(t *testing.T) {
	for _, tc := range []struct {
		kind, spec, liveSpec, secretField string
		prefix                            apid.Prefix
	}{
		{"Key", `{"keyData":{"value":"low-entropy-secret"}}`, `{"usage":"data_encryption","materialType":"symmetric","desiredState":"active"}`, "keyData", apid.PrefixKey},
		{"Actor", `{"signingKey":{"sharedKey":{"value":"low-entropy-secret"}}}`, `{"externalId":"subject"}`, "signingKey", apid.PrefixActor},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			doc := clientDoc(t, tc.kind, "  name: example\n  namespace: root", tc.spec)
			live := reconcileLive(t, tc.kind, apid.New(tc.prefix).String(), tc.liveSpec, nil)
			plan, err := Reconcile(Target{doc, live}, ReconcileOptions{Overwrite: true})
			require.NoError(t, err)
			object := planObject(t, plan)
			raw := object["metadata"].(map[string]any)["annotations"].(map[string]any)[LastAppliedAnnotation].(string)
			require.NotContains(t, raw, "low-entropy-secret")
			require.NotContains(t, raw, "***")
			require.Contains(t, raw, "/spec/"+tc.secretField)
			require.Contains(t, object["spec"], tc.secretField)
			live = withHistory(t, live, doc)
			omitted := clientDoc(t, tc.kind, "  name: example\n  namespace: root", "{}")
			plan, err = Reconcile(Target{omitted, live}, ReconcileOptions{Overwrite: true})
			require.NoError(t, err)
			if plan.Operation == OperationUpdate {
				require.NotContains(t, planObject(t, plan)["spec"], tc.secretField)
			}
			clear := clientDoc(t, tc.kind, "  name: example\n  namespace: root", fmt.Sprintf(`{"%s":null}`, tc.secretField))
			plan, err = Reconcile(Target{clear, live}, ReconcileOptions{Overwrite: true})
			if tc.kind == "Key" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Contains(t, planObject(t, plan)["spec"], tc.secretField)
			}
		})
	}
}

func TestReconcileCreateAndHistoryErrors(t *testing.T) {
	doc := clientDoc(t, "Key", "  name: example\n  namespace: root", `{"keyData":{"value":"test-secret"}}`)
	plan, err := Reconcile(Target{Document: doc}, ReconcileOptions{Overwrite: true})
	require.NoError(t, err)
	require.Equal(t, OperationCreate, plan.Operation)
	require.NotContains(t, plan.Document.Metadata.Annotations[LastAppliedAnnotation], "test-secret")
	require.Contains(t, plan.Document.Object["spec"].(map[string]any)["keyData"], "value")
	live := reconcileLive(t, "Key", apid.New(apid.PrefixKey).String(), `{"usage":"data_encryption","materialType":"symmetric"}`, nil)
	for _, raw := range []string{"garbage secret", `{"version":999,"desired":{}}`, `{"version":1,"version":1,"desired":{}}`, strings.Repeat("x", meta.AnnotationsTotalMaxSize+1)} {
		live.Metadata.Annotations = map[string]string{LastAppliedAnnotation: raw}
		_, err := Reconcile(Target{doc, live}, ReconcileOptions{Overwrite: true})
		require.Error(t, err)
		require.NotContains(t, err.Error(), "garbage secret")
	}
	big := clientDoc(t, "Actor", "  name: example\n  namespace: root\n  annotations: {big: "+strings.Repeat("x", 140000)+"}", `{"externalId":"subject"}`)
	_, err = Reconcile(Target{Document: big}, ReconcileOptions{Overwrite: true})
	require.ErrorContains(t, err, "256 KiB")
}

func TestReconcileArraysAreAtomic(t *testing.T) {
	m := threeWay{overwrite: true}
	old := []any{"a", "b"}
	live := []any{"a", "b", "external"}
	desired := []any{}
	value, present, err := m.merge(old, true, live, true, desired, true, []string{"spec", "array"})
	require.NoError(t, err)
	require.True(t, present)
	require.Equal(t, desired, value)
	_, _, err = (threeWay{}).merge(old, true, live, true, desired, true, []string{"spec", "array"})
	require.ErrorContains(t, err, "conflict")
}

func TestReconcileFailedWriteDoesNotAdvanceHistory(t *testing.T) {
	live := reconcileLive(t, "Actor", apid.New(apid.PrefixActor).String(), `{"externalId":"subject"}`, nil)
	doc := clientDoc(t, "Actor", "  name: example\n  namespace: root", "{}")
	plan, err := Reconcile(Target{doc, live}, ReconcileOptions{Overwrite: true})
	require.NoError(t, err)
	var requests int
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		require.Equal(t, "PATCH", r.Method)
		w.WriteHeader(409)
	}, false)
	_, err = c.Update(context.Background(), plan.Target, plan.Patch)
	require.Error(t, err)
	require.Equal(t, 1, requests)
	require.Empty(t, live.Metadata.Annotations)
}

func TestReconcileEmptyListBecomesUnchanged(t *testing.T) {
	doc := clientDoc(t, "Actor", "  name: example\n  namespace: root", `{"permissions":[]}`)
	live := withHistory(t, reconcileLive(t, "Actor", apid.New(apid.PrefixActor).String(), `{"externalId":"subject"}`, nil), doc)
	plan, err := Reconcile(Target{doc, live}, ReconcileOptions{Overwrite: true})
	require.NoError(t, err)
	require.Equal(t, OperationUnchanged, plan.Operation)
}

func TestReconcileNestedSecretsAndRedactedReplacement(t *testing.T) {
	spec := `{"definition":{"displayName":"Example","auth":{"type":"OAuth2","clientId":{"value":"client"},"clientSecret":{"value":"confidential-material"},"authorization":{"endpoint":"https://example.com/auth"},"token":{"endpoint":"https://example.com/token"}}}}`
	doc := clientDoc(t, "Connector", "  name: example\n  namespace: root", spec)
	h, err := newHistory(doc)
	require.NoError(t, err)
	data, err := json.Marshal(h)
	require.NoError(t, err)
	require.NotContains(t, string(data), "confidential-material")
	require.Contains(t, h.Secrets, "/spec/definition/auth/clientSecret")
	_, err = readHistory(string(data), "Connector")
	require.NoError(t, err)
	live := withHistory(t, reconcileLive(t, "Connector", apid.New(apid.PrefixConnector).String(), strings.ReplaceAll(spec, "confidential-material", "********"), nil), doc)
	live.Redacted = true
	// Omitted masked secrets do not prevent metadata-only writes.
	metadataOnly := clientDoc(t, "Connector", "  name: example\n  namespace: root\n  labels: {env: prod}", strings.Replace(spec, `"clientSecret":{"value":"confidential-material"},`, "", 1))
	plan, err := Reconcile(Target{metadataOnly, live}, ReconcileOptions{Overwrite: true})
	require.NoError(t, err)
	require.Empty(t, planObject(t, plan)["spec"])
	// Replacing a definition cannot preserve an unknown masked credential.
	changed := clientDoc(t, "Connector", "  name: example\n  namespace: root", strings.Replace(strings.Replace(spec, `"clientSecret":{"value":"confidential-material"},`, "", 1), "Example", "Changed", 1))
	_, err = Reconcile(Target{changed, live}, ReconcileOptions{Overwrite: true})
	require.ErrorContains(t, err, "redacted placeholders")
	// Supplying the credential allows the replacement; history still excludes it.
	supplied := clientDoc(t, "Connector", "  name: example\n  namespace: root", strings.Replace(spec, "Example", "Changed", 1))
	plan, err = Reconcile(Target{supplied, live}, ReconcileOptions{Overwrite: true})
	require.NoError(t, err)
	require.NotContains(t, planObject(t, plan)["metadata"].(map[string]any)["annotations"].(map[string]any)[LastAppliedAnnotation], "confidential-material")
}

func TestHistoryRejectsEmbeddedSecretsAndWrongIdentity(t *testing.T) {
	doc := clientDoc(t, "Key", "  name: example\n  namespace: root", `{"keyData":{"value":"do-not-echo"}}`)
	raw, err := json.Marshal(History{Version: 1, Desired: doc.Object})
	require.NoError(t, err)
	_, err = readHistory(string(raw), "Key")
	require.ErrorContains(t, err, "excluded fields")
	require.NotContains(t, err.Error(), "do-not-echo")
	live := reconcileLive(t, "Key", apid.New(apid.PrefixKey).String(), `{"usage":"data_encryption"}`, nil)
	h, err := newHistory(doc)
	require.NoError(t, err)
	h.Desired["metadata"].(map[string]any)["id"] = apid.New(apid.PrefixKey).String()
	raw, err = json.Marshal(h)
	require.NoError(t, err)
	live.Metadata.Annotations = map[string]string{LastAppliedAnnotation: string(raw)}
	_, err = Reconcile(Target{doc, live}, ReconcileOptions{Overwrite: true})
	require.ErrorContains(t, err, "different resource")
}

func TestReconcileEveryCreateContract(t *testing.T) {
	for _, tc := range []struct{ kind, spec string }{
		{"Namespace", `{}`},
		{"Actor", `{"externalId":"subject"}`},
		{"Key", `{"keyData":{"value":"fake-secret"}}`},
		{"Connector", `{"definition":{"displayName":"Example","auth":{"type":"no-auth"}}}`},
		{"RateLimit", `{"algorithm":{"tokenBucket":{"capacity":10,"refillRate":1}}}`},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			doc := clientDoc(t, tc.kind, "  name: example\n  namespace: root", tc.spec)
			plan, err := Reconcile(Target{Document: doc}, ReconcileOptions{Overwrite: true})
			require.NoError(t, err)
			require.Equal(t, OperationCreate, plan.Operation)
			h, err := readHistory(plan.Document.Metadata.Annotations[LastAppliedAnnotation], doc.Kind)
			require.NoError(t, err)
			require.Equal(t, 1, h.Version)
		})
	}
}

func TestReconcileRejectsPlaceholdersWithoutLoader(t *testing.T) {
	doc := clientDoc(t, "Key", "  name: example\n  namespace: root", `{"keyData":{"value":"real-value"}}`)
	doc.Object["spec"].(map[string]any)["keyData"].(map[string]any)["value"] = "*****"
	_, err := Reconcile(Target{Document: doc}, ReconcileOptions{Overwrite: true})
	require.ErrorContains(t, err, "redacted placeholders")
}

func TestHistoryPreservesLargeGenerationNumbers(t *testing.T) {
	doc := clientDoc(t, "Connector", "  name: example\n  namespace: root\n  generation: 9007199254740993", `{}`)
	history, err := newHistory(doc)
	require.NoError(t, err)
	encoded, err := json.Marshal(history)
	require.NoError(t, err)
	decoded, err := readHistory(string(encoded), "Connector")
	require.NoError(t, err)
	roundTrip, err := json.Marshal(decoded)
	require.NoError(t, err)
	require.JSONEq(t, string(encoded), string(roundTrip))
	require.Contains(t, string(roundTrip), "9007199254740993")
}
