package apply

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/stretchr/testify/require"
)

func TestEveryAdapterCreateAndMetadataUpdate(t *testing.T) {
	cases := []struct {
		kind, collection, spec string
		prefix                 apid.Prefix
	}{
		{"Namespace", "namespaces", `{}`, ""},
		{"Actor", "actors", `{"externalId":"subject"}`, apid.PrefixActor},
		{"Key", "keys", `{"keyData":{"value":"test-key"}}`, apid.PrefixKey},
		{"Connector", "connectors", `{"definition":{"displayName":"Example","auth":{"type":"no-auth"}}}`, apid.PrefixConnector},
		{"RateLimit", "rate-limits", `{"algorithm":{"tokenBucket":{"capacity":10,"refillRate":1}}}`, apid.PrefixRateLimit},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			id := "root.example"
			if tc.prefix != "" {
				id = apid.New(tc.prefix).String()
			}
			var methods []string
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				methods = append(methods, r.Method)
				if r.Method == "POST" {
					require.Equal(t, "/api/v1/"+tc.collection, r.URL.Path)
				} else {
					require.Equal(t, "/api/v1/"+tc.collection+"/"+id, r.URL.Path)
				}
				fmt.Fprint(w, liveJSON(tc.kind, id, "example", "root", tc.spec))
			}, true)
			doc := clientDoc(t, tc.kind, "  name: example\n  namespace: root", tc.spec)
			live, err := c.Create(context.Background(), doc)
			require.NoError(t, err)
			patch := []byte(fmt.Sprintf(`{"apiVersion":"authproxy.net/v1alpha1","kind":%q,"metadata":{"labels":{"env":"prod"}},"spec":{}}`, tc.kind))
			_, err = c.Update(context.Background(), Target{Document: doc, Current: live}, patch)
			require.NoError(t, err)
			require.Equal(t, []string{"POST", "PATCH"}, methods)
		})
	}
}

func TestConnectionMetadataOnly(t *testing.T) {
	id := apid.New(apid.PrefixConnection).String()
	connectorID := apid.New(apid.PrefixConnector).String()
	response := fmt.Sprintf(`{"apiVersion":"authproxy.net/v1alpha1","kind":"Connection","metadata":{"id":%q,"name":"example","namespace":"root","createdAt":"2026-09-01T00:00:00Z","updatedAt":"2026-09-01T00:00:00Z"},"spec":{"connectorRef":{"apiVersion":"authproxy.net/v1alpha1","kind":"Connector","id":%q,"generation":1}},"status":{"lifecycle":{"state":"configured"},"health":{"state":"healthy"},"configuration":{"configured":true,"schema":{"type":"object"}}}}`, id, connectorID)
	var methods []string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		fmt.Fprint(w, response)
	}, false)
	doc := clientDoc(t, "Connection", "  id: "+id, "{}")
	targets, err := c.ResolveBatch(context.Background(), []Document{doc})
	require.NoError(t, err)
	patch := []byte(`{"apiVersion":"authproxy.net/v1alpha1","kind":"Connection","metadata":{"labels":{}},"spec":{}}`)
	_, err = c.Update(context.Background(), targets[0], patch)
	require.NoError(t, err)
	invalid := []byte(`{"apiVersion":"authproxy.net/v1alpha1","kind":"Connection","metadata":{},"spec":{"configuration":{"token":"secret"}}}`)
	_, err = c.Update(context.Background(), targets[0], invalid)
	require.Error(t, err)
	_, err = c.Create(context.Background(), doc)
	require.ErrorContains(t, err, "setup actions")
	require.Equal(t, []string{"GET", "PATCH"}, methods)
}

func TestPatchPresenceAndImmutableKeyPolicy(t *testing.T) {
	id := apid.New(apid.PrefixKey).String()
	var calls int
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprint(w, liveJSON("Key", id, "example", "root", `{"usage":"data_encryption","materialType":"symmetric","desiredState":"active"}`))
	}, true)
	doc := clientDoc(t, "Key", "  id: "+id, "{}")
	live, err := c.Resolve(context.Background(), doc)
	require.NoError(t, err)
	require.Equal(t, []string{"spec.keyData"}, live.WriteOnlyFields)
	for _, spec := range []string{`{"keyData":null}`, `{"keyData":{"value":"********"}}`, `{"materialType":"private"}`, `{"desiredState":"not-valid"}`} {
		patch := []byte(fmt.Sprintf(`{"apiVersion":"authproxy.net/v1alpha1","kind":"Key","metadata":{},"spec":%s}`, spec))
		_, err = c.Update(context.Background(), Target{Document: doc, Current: live}, patch)
		require.Error(t, err)
	}
	require.Equal(t, 1, calls)
	// Nullable Namespace references preserve an explicit clear through the typed patch.
	nsLive, err := c.decodeLive("Namespace", []byte(liveJSON("Namespace", "root.example", "example", "root", `{}`)), false)
	require.NoError(t, err)
	patch, err := decodePatch("Namespace", []byte(`{"apiVersion":"authproxy.net/v1alpha1","kind":"Namespace","metadata":{"annotations":{}},"spec":{"encryptionKeyRef":null}}`), nsLive.Resource)
	require.NoError(t, err)
	data, err := json.Marshal(patch)
	require.NoError(t, err)
	require.Contains(t, string(data), `"encryptionKeyRef":null`)
}
