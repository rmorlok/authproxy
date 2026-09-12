package apply

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apauth/jwt"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apserde"
	"github.com/stretchr/testify/require"
)

func clientDoc(t *testing.T, kind, metadata, spec string) Document {
	t.Helper()
	docs, err := loadText(t, manifestText(kind, metadata, spec), "")
	require.NoError(t, err)
	return docs[0]
}
func testClient(t *testing.T, handler http.HandlerFunc, admin bool) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	c, err := NewClient(ClientOptions{APIURL: server.URL, AdminAPIURL: server.URL, Admin: admin, Signer: jwt.NewSigner("test-token"), Timeout: time.Second})
	require.NoError(t, err)
	return c
}
func liveJSON(kind, id, name, ns, spec string) string {
	return fmt.Sprintf(`{"apiVersion":"authproxy.net/v1alpha1","kind":%q,"metadata":{"id":%q,"name":%q,"namespace":%q},"spec":%s}`, kind, id, name, ns, spec)
}
func actorJSON(id, name, ns string) string {
	return liveJSON("Actor", id, name, ns, `{"externalId":"external-bob"}`)
}
func listJSON(kind string, items []string, cursor string) string {
	return fmt.Sprintf(`{"apiVersion":"authproxy.net/v1alpha1","kind":%q,"metadata":{"continue":%q},"items":[%s]}`, kind+"List", cursor, strings.Join(items, ","))
}

func TestResolveNamespacedNameAcrossPages(t *testing.T) {
	id := apid.New(apid.PrefixActor).String()
	var calls int
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		require.Equal(t, "/api/v1/actors", r.URL.Path)
		calls++
		if calls == 1 {
			require.Equal(t, "root.prod", r.URL.Query().Get("namespace"))
			require.Equal(t, "bob", r.URL.Query().Get("name"))
			fmt.Fprint(w, listJSON("Actor", []string{actorJSON(apid.New(apid.PrefixActor).String(), "bob", "root.prod.child")}, "next+cursor"))
		} else {
			require.Equal(t, "next+cursor", r.URL.Query().Get("cursor"))
			require.Len(t, r.URL.Query(), 1)
			fmt.Fprint(w, listJSON("Actor", []string{actorJSON(id, "bob", "root.prod")}, ""))
		}
	}, false)
	doc := clientDoc(t, "Actor", "  name: bob\n  namespace: root.prod", "{}")
	targets, err := c.ResolveBatch(context.Background(), []Document{doc})
	require.NoError(t, err)
	require.Len(t, targets, 1)
	require.Equal(t, id, targets[0].Current.Metadata.ID)
	require.Equal(t, 2, calls)
	require.Equal(t, []string{"spec.signingKey"}, targets[0].Current.WriteOnlyFields)
}

func TestResolveIDConsistencyAndMissingIDs(t *testing.T) {
	id := apid.New(apid.PrefixActor).String()
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, id) {
			fmt.Fprint(w, actorJSON(id, "bob", "root.prod"))
			return
		}
		http.Error(w, "secret server message", http.StatusNotFound)
	}, false)
	doc := clientDoc(t, "Actor", "  id: "+id, "{}")
	live, err := c.Resolve(context.Background(), doc)
	require.NoError(t, err)
	require.Equal(t, "root.prod", live.Metadata.Namespace)
	for _, metadata := range []string{"  id: " + id + "\n  name: other", "  id: " + id + "\n  namespace: root.other"} {
		_, err := c.Resolve(context.Background(), clientDoc(t, "Actor", metadata, "{}"))
		require.ErrorContains(t, err, "does not match")
	}
	_, err = c.Resolve(context.Background(), clientDoc(t, "Actor", "  id: "+apid.New(apid.PrefixActor).String(), "{}"))
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, 404, apiErr.StatusCode)
	require.NotContains(t, err.Error(), "secret")
}

func TestResolveBatchAliases(t *testing.T) {
	id := apid.New(apid.PrefixActor).String()
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/actors" {
			fmt.Fprint(w, listJSON("Actor", []string{actorJSON(id, "bob", "root")}, ""))
		} else {
			fmt.Fprint(w, actorJSON(id, "bob", "root"))
		}
	}, false)
	doc := clientDoc(t, "Actor", "  name: bob\n  namespace: root", "{}")
	byID := clientDoc(t, "Actor", "  id: "+id, "{}")
	targets, err := c.ResolveBatch(context.Background(), []Document{doc, byID})
	require.Nil(t, targets)
	require.ErrorContains(t, err, "duplicate live target")
}

func TestKeyOperationsOnBothServices(t *testing.T) {
	for _, admin := range []bool{false, true} {
		t.Run(fmt.Sprintf("admin=%t", admin), func(t *testing.T) {
			id := apid.New(apid.PrefixKey).String()
			response := liveJSON("Key", id, "example", "root", `{"usage":"data_encryption","materialType":"symmetric","desiredState":"active"}`)
			var requests []string
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r.Method+" "+r.URL.Path)
				if r.Method == "GET" && r.URL.Path == "/api/v1/keys" {
					fmt.Fprint(w, listJSON("Key", []string{response}, ""))
				} else {
					fmt.Fprint(w, response)
				}
			}, admin)
			doc := clientDoc(t, "Key", "  name: example\n  namespace: root", "{}")
			targets, err := c.ResolveBatch(context.Background(), []Document{doc})
			require.NoError(t, err)
			require.Equal(t, id, targets[0].Current.Metadata.ID)
			byID := clientDoc(t, "Key", "  id: "+id, "{}")
			_, err = c.Resolve(context.Background(), byID)
			require.NoError(t, err)
			create := clientDoc(t, "Key", "  name: example\n  namespace: root", "{keyData: {value: test-key}}")
			_, err = c.Create(context.Background(), create)
			require.NoError(t, err)
			patch := []byte(`{"apiVersion":"authproxy.net/v1alpha1","kind":"Key","metadata":{"labels":{"env":"prod"}},"spec":{}}`)
			_, err = c.Update(context.Background(), targets[0], patch)
			require.NoError(t, err)
			require.Equal(t, []string{"GET /api/v1/keys", "GET /api/v1/keys/" + id, "POST /api/v1/keys", "PATCH /api/v1/keys/" + id}, requests)
		})
	}
}

func TestKeyAPIPermissionDenied(t *testing.T) {
	var calls int
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"sensitive server detail"}`)
	}, false)
	doc := clientDoc(t, "Key", "  name: example\n  namespace: root", "{}")
	_, err := c.ResolveBatch(context.Background(), []Document{doc})
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusForbidden, apiErr.StatusCode)
	require.NotContains(t, err.Error(), "sensitive")
	require.Equal(t, 1, calls)
}

func TestMissingResourcesAndCreateValidation(t *testing.T) {
	var writes int
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			writes++
		}
		if strings.HasPrefix(r.URL.Path, "/api/v1/namespaces/") {
			w.WriteHeader(404)
			return
		}
		collection := strings.TrimPrefix(r.URL.Path, "/api/v1/")
		kind := map[string]string{"actors": "Actor", "connections": "Connection", "connectors": "Connector"}[collection]
		fmt.Fprint(w, listJSON(kind, nil, ""))
	}, false)
	valid := clientDoc(t, "Actor", "  name: bob\n  namespace: root", "{externalId: external-bob}")
	targets, err := c.ResolveBatch(context.Background(), []Document{valid})
	require.NoError(t, err)
	require.Nil(t, targets[0].Current)
	invalid := clientDoc(t, "Actor", "  name: bob\n  namespace: root", "{}")
	_, err = c.ResolveBatch(context.Background(), []Document{invalid})
	require.ErrorContains(t, err, "semantic")
	_, err = c.ResolveBatch(context.Background(), []Document{clientDoc(t, "Connection", "  name: cxn\n  namespace: root", "{}")})
	require.ErrorContains(t, err, "not found")
	_, err = c.Resolve(context.Background(), clientDoc(t, "Connector", "  name: cxr\n  namespace: root\n  generation: 3", "{}"))
	require.ErrorContains(t, err, "not found")
	targets, err = c.ResolveBatch(context.Background(), []Document{clientDoc(t, "Namespace", "  name: prod\n  namespace: root", "{}")})
	require.NoError(t, err)
	require.Nil(t, targets[0].Current)
	_, err = c.Resolve(context.Background(), clientDoc(t, "Namespace", "  id: root.prod", "{}"))
	require.Error(t, err)
	require.Zero(t, writes)
}

func TestPaginationErrors(t *testing.T) {
	for _, scenario := range []string{"cycle", "ambiguous", "invalid-list", "wrong-kind"} {
		t.Run(scenario, func(t *testing.T) {
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				switch scenario {
				case "cycle":
					fmt.Fprint(w, listJSON("Actor", nil, "same"))
				case "ambiguous":
					fmt.Fprint(w, listJSON("Actor", []string{actorJSON(apid.New(apid.PrefixActor).String(), "bob", "root"), actorJSON(apid.New(apid.PrefixActor).String(), "bob", "root")}, ""))
				case "invalid-list":
					fmt.Fprint(w, `{}`)
				case "wrong-kind":
					fmt.Fprint(w, listJSON("Actor", []string{liveJSON("Key", apid.New(apid.PrefixKey).String(), "bob", "root", `{}`)}, ""))
				}
			}, false)
			_, err := c.Resolve(context.Background(), clientDoc(t, "Actor", "  name: bob\n  namespace: root", "{}"))
			require.Error(t, err)
		})
	}
}

func TestHTTPFailuresTimeoutCancellationAndRedirect(t *testing.T) {
	for _, status := range []int{401, 403, 409, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var calls int
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) { calls++; http.Error(w, "must-not-leak", status) }, false)
			_, err := c.Resolve(context.Background(), clientDoc(t, "Actor", "  name: bob\n  namespace: root", "{}"))
			var apiErr *APIError
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, status, apiErr.StatusCode)
			require.NotContains(t, err.Error(), "must-not-leak")
			require.Equal(t, 1, calls)
		})
	}
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }, false)
	c.http.Timeout = 10 * time.Millisecond
	_, err := c.Resolve(context.Background(), clientDoc(t, "Actor", "  name: bob\n  namespace: root", "{}"))
	require.ErrorIs(t, err, context.DeadlineExceeded)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = c.Resolve(ctx, clientDoc(t, "Actor", "  name: bob\n  namespace: root", "{}"))
	require.ErrorIs(t, err, context.Canceled)
	var destinationCalls atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { destinationCalls.Add(1) }))
	defer destination.Close()
	redirect := testClient(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, 302) }, false)
	_, err = redirect.Resolve(context.Background(), clientDoc(t, "Actor", "  name: bob\n  namespace: root", "{}"))
	require.Error(t, err)
	require.Zero(t, destinationCalls.Load())
}

func TestCreateAndPatchHTTPContracts(t *testing.T) {
	id := apid.New(apid.PrefixActor).String()
	var methods []string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		methods = append(methods, r.Method)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var value map[string]any
		require.NoError(t, json.Unmarshal(body, &value))
		if r.Method == "POST" {
			require.Equal(t, "/api/v1/actors", r.URL.Path)
			require.NotContains(t, value["metadata"], "id")
		} else {
			require.Equal(t, "/api/v1/actors/"+id, r.URL.Path)
			require.Equal(t, map[string]any{}, value["metadata"].(map[string]any)["labels"])
			require.NotContains(t, value["spec"], "signingKey")
		}
		fmt.Fprint(w, actorJSON(id, "bob", "root"))
	}, false)
	doc := clientDoc(t, "Actor", "  name: bob\n  namespace: root", "{externalId: external-bob}")
	live, err := c.Create(context.Background(), doc)
	require.NoError(t, err)
	patch := []byte(`{"apiVersion":"authproxy.net/v1alpha1","kind":"Actor","metadata":{"labels":{}},"spec":{}}`)
	_, err = c.Update(context.Background(), Target{Document: doc, Current: live}, patch)
	require.NoError(t, err)
	require.Equal(t, []string{"POST", "PATCH"}, methods)
	invalid := []byte(`{"apiVersion":"authproxy.net/v1alpha1","kind":"Actor","metadata":{},"spec":{"externalId":"different"}}`)
	_, err = c.Update(context.Background(), Target{Document: doc, Current: live}, invalid)
	require.ErrorContains(t, err, "immutable")
	require.Len(t, methods, 2)
}

func TestRedactionMetadataAndExplicitGeneration(t *testing.T) {
	id := apid.New(apid.PrefixConnector).String()
	var paths []string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set(apserde.RedactedHeader, "true")
		fmt.Fprintf(w, `{"apiVersion":"authproxy.net/v1alpha1","kind":"Connector","metadata":{"id":%q,"name":"example","namespace":"root","generation":3},"spec":{}}`, id)
	}, false)
	doc := clientDoc(t, "Connector", "  id: "+id+"\n  generation: 3", "{}")
	live, err := c.Resolve(context.Background(), doc)
	require.NoError(t, err)
	require.True(t, live.Redacted)
	require.Equal(t, []string{"/api/v1/connectors/" + id, "/api/v1/connectors/" + id + "/generations/3"}, paths)
	patch := []byte(`{"apiVersion":"authproxy.net/v1alpha1","kind":"Connector","metadata":{},"spec":{}}`)
	_, err = c.Update(context.Background(), Target{Document: doc, Current: live}, patch)
	require.NoError(t, err)
	require.Equal(t, "/api/v1/connectors/"+id+"/generations/3", paths[2])
}

func TestClientOptions(t *testing.T) {
	for _, options := range []ClientOptions{
		{}, {APIURL: "http://example", Admin: true, Signer: jwt.NewSigner("token")},
		{APIURL: "http://user:secret@example", Signer: jwt.NewSigner("token")},
		{APIURL: "http://example?token=secret", Signer: jwt.NewSigner("token")},
		{APIURL: "http://example", Signer: jwt.NewSigner("token"), Timeout: -1},
	} {
		_, err := NewClient(options)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "secret")
	}
}

// Ensure API errors remain useful to the future executor without body replay.
var _ error = (*APIError)(nil)

func TestIncompletePageAndBodyTimeout(t *testing.T) {
	doc := clientDoc(t, "Actor", "  name: bob\n  namespace: root", "{}")
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"apiVersion":"authproxy.net/v1alpha1","kind":"ActorList","metadata":{"remainingItemCount":1},"items":[]}`)
	}, false)
	_, err := c.Resolve(context.Background(), doc)
	require.ErrorContains(t, err, "incomplete")
	c = testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}, false)
	c.http.Timeout = 10 * time.Millisecond
	_, err = c.Resolve(context.Background(), doc)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestFailedMutationIsNotRetried(t *testing.T) {
	calls := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(http.StatusConflict) }, false)
	doc := clientDoc(t, "Actor", "  name: bob\n  namespace: root", "{externalId: external-bob}")
	_, err := c.Create(context.Background(), doc)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, 409, apiErr.StatusCode)
	require.Equal(t, 1, calls)
}
