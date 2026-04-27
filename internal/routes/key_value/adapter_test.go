package key_value

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	authcore "github.com/rmorlok/authproxy/internal/apauth/core"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apgin"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httperr"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeResource is a minimal Resource that also satisfies the auth
// validator's runtime expectations (GetNamespace + GetId).
type fakeResource struct {
	id          apid.ID
	namespace   string
	labels      map[string]string
	annotations map[string]string
}

func (f *fakeResource) GetId() apid.ID                   { return f.id }
func (f *fakeResource) GetNamespace() string             { return f.namespace }
func (f *fakeResource) GetLabels() map[string]string     { return f.labels }
func (f *fakeResource) GetAnnotations() map[string]string {
	return f.annotations
}

// store is an in-memory, mutex-guarded backing store for the adapter
// closures.
type store struct {
	mu        sync.Mutex
	resources map[apid.ID]*fakeResource
}

func newStore() *store {
	return &store{resources: make(map[apid.ID]*fakeResource)}
}

func (s *store) put(r *fakeResource) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resources[r.id] = r
}

func (s *store) get(id apid.ID) (*fakeResource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.resources[id]
	if !ok {
		return nil, database.ErrNotFound
	}
	return r, nil
}

// permissiveAuth installs a ResourcePermissionValidator with wildcard
// permissions onto the gin context so handlers can call
// MustGetValidatorFromGinContext.
func permissiveAuth() gin.HandlerFunc {
	return func(gctx *gin.Context) {
		ra := authcore.NewAuthenticatedRequestAuth(&authcore.Actor{
			Id:          apid.New(apid.PrefixActor),
			ExternalId:  "test-actor",
			Namespace:   "root",
			Permissions: aschema.AllPermissions(),
		})

		val := auth.NewResourcePermissionValidatorForTest(ra, "fakes", "edit")
		ctx := val.ContextWith(gctx.Request.Context())
		gctx.Request = gctx.Request.WithContext(ctx)
		gctx.Next()
	}
}

func parseFakeID(gctx *gin.Context) (apid.ID, *httperr.Error) {
	raw := gctx.Param("id")
	id, err := apid.Parse(raw)
	if err != nil {
		return apid.Nil, httperr.BadRequestf("invalid id: %s", err.Error())
	}
	return id, nil
}

type adapterFixture struct {
	store   *store
	engine  *gin.Engine
	adapter Adapter[apid.ID]
}

func newAdapterFixture(t *testing.T, kind Kind) *adapterFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	s := newStore()

	a := Adapter[apid.ID]{
		Kind:         kind,
		ResourceName: "fake",
		PathPrefix:   "/fakes/:id",
		AuthGet:      permissiveAuth(),
		AuthMutate:   permissiveAuth(),
		ParseID:      parseFakeID,
		Get: func(_ context.Context, id apid.ID) (Resource, error) {
			r, err := s.get(id)
			if err != nil {
				return nil, err
			}
			return r, nil
		},
		Put: func(_ context.Context, id apid.ID, kv map[string]string) (Resource, error) {
			r, err := s.get(id)
			if err != nil {
				return nil, err
			}
			target := kind.Get(r)
			if target == nil {
				target = make(map[string]string)
				if kind.PathSegment == "labels" {
					r.labels = target
				} else {
					r.annotations = target
				}
			}
			for k, v := range kv {
				target[k] = v
			}
			return r, nil
		},
		Delete: func(_ context.Context, id apid.ID, keys []string) (Resource, error) {
			r, err := s.get(id)
			if err != nil {
				return nil, err
			}
			target := kind.Get(r)
			for _, k := range keys {
				delete(target, k)
			}
			return r, nil
		},
	}

	engine := apgin.ForTest(nil)
	a.Register(engine)

	return &adapterFixture{store: s, engine: engine, adapter: a}
}

func (f *adapterFixture) do(method, path string, body io.Reader) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, body)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	f.engine.ServeHTTP(w, req)
	return w
}

func newFake(id apid.ID) *fakeResource {
	return &fakeResource{
		id:          id,
		namespace:   "root",
		labels:      map[string]string{},
		annotations: map[string]string{},
	}
}

func TestAdapter_Register_RoutesEachVerb(t *testing.T) {
	f := newAdapterFixture(t, Label)

	got := make(map[string]bool)
	for _, ri := range f.engine.Routes() {
		got[ri.Method+" "+ri.Path] = true
	}

	for _, want := range []string{
		"GET /fakes/:id/labels",
		"GET /fakes/:id/labels/:label",
		"PUT /fakes/:id/labels/:label",
		"DELETE /fakes/:id/labels/:label",
	} {
		assert.True(t, got[want], "expected route %q to be registered", want)
	}
}

func TestAdapter_HandleList(t *testing.T) {
	t.Run("returns labels map", func(t *testing.T) {
		f := newAdapterFixture(t, Label)
		id := apid.New(apid.PrefixActor)
		r := newFake(id)
		r.labels["env"] = "prod"
		r.labels["team"] = "core"
		f.store.put(r)

		w := f.do(http.MethodGet, "/fakes/"+string(id)+"/labels", nil)
		require.Equal(t, http.StatusOK, w.Code)

		var got map[string]string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.Equal(t, map[string]string{"env": "prod", "team": "core"}, got)
	})

	t.Run("returns empty map when nil", func(t *testing.T) {
		f := newAdapterFixture(t, Label)
		id := apid.New(apid.PrefixActor)
		r := newFake(id)
		r.labels = nil
		f.store.put(r)

		w := f.do(http.MethodGet, "/fakes/"+string(id)+"/labels", nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `{}`, w.Body.String())
	})

	t.Run("404 when resource missing", func(t *testing.T) {
		f := newAdapterFixture(t, Label)
		w := f.do(http.MethodGet, "/fakes/"+string(apid.New(apid.PrefixActor))+"/labels", nil)
		require.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), "fake not found")
	})

	t.Run("annotations route works on Annotation kind", func(t *testing.T) {
		f := newAdapterFixture(t, Annotation)
		id := apid.New(apid.PrefixActor)
		r := newFake(id)
		r.annotations["note"] = "hello"
		f.store.put(r)

		w := f.do(http.MethodGet, "/fakes/"+string(id)+"/annotations", nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `{"note":"hello"}`, w.Body.String())
	})
}

func TestAdapter_HandleGet(t *testing.T) {
	t.Run("returns key/value pair", func(t *testing.T) {
		f := newAdapterFixture(t, Label)
		id := apid.New(apid.PrefixActor)
		r := newFake(id)
		r.labels["env"] = "prod"
		f.store.put(r)

		w := f.do(http.MethodGet, "/fakes/"+string(id)+"/labels/env", nil)
		require.Equal(t, http.StatusOK, w.Code)

		var got KeyValueJson
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.Equal(t, KeyValueJson{Key: "env", Value: "prod"}, got)
	})

	t.Run("404 when key missing", func(t *testing.T) {
		f := newAdapterFixture(t, Label)
		id := apid.New(apid.PrefixActor)
		f.store.put(newFake(id))

		w := f.do(http.MethodGet, "/fakes/"+string(id)+"/labels/missing", nil)
		require.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), "label 'missing' not found")
	})

	t.Run("404 when resource missing", func(t *testing.T) {
		f := newAdapterFixture(t, Label)
		w := f.do(http.MethodGet, "/fakes/"+string(apid.New(apid.PrefixActor))+"/labels/env", nil)
		require.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestAdapter_HandlePut(t *testing.T) {
	t.Run("upserts value and returns it", func(t *testing.T) {
		f := newAdapterFixture(t, Label)
		id := apid.New(apid.PrefixActor)
		f.store.put(newFake(id))

		w := f.do(http.MethodPut, "/fakes/"+string(id)+"/labels/env",
			strings.NewReader(`{"value":"prod"}`))
		require.Equal(t, http.StatusOK, w.Code)

		var got KeyValueJson
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.Equal(t, KeyValueJson{Key: "env", Value: "prod"}, got)

		stored, _ := f.store.get(id)
		assert.Equal(t, "prod", stored.labels["env"])
	})

	t.Run("400 when key invalid", func(t *testing.T) {
		f := newAdapterFixture(t, Label)
		id := apid.New(apid.PrefixActor)
		f.store.put(newFake(id))

		w := f.do(http.MethodPut, "/fakes/"+string(id)+"/labels/-bad",
			strings.NewReader(`{"value":"prod"}`))
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "invalid label key")
	})

	t.Run("400 when value invalid", func(t *testing.T) {
		f := newAdapterFixture(t, Label)
		id := apid.New(apid.PrefixActor)
		f.store.put(newFake(id))

		w := f.do(http.MethodPut, "/fakes/"+string(id)+"/labels/env",
			strings.NewReader(`{"value":"!!!"}`))
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "invalid label value")
	})

	t.Run("400 when body malformed", func(t *testing.T) {
		f := newAdapterFixture(t, Label)
		id := apid.New(apid.PrefixActor)
		f.store.put(newFake(id))

		w := f.do(http.MethodPut, "/fakes/"+string(id)+"/labels/env",
			strings.NewReader(`not-json`))
		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("404 when resource missing", func(t *testing.T) {
		f := newAdapterFixture(t, Label)
		w := f.do(http.MethodPut, "/fakes/"+string(apid.New(apid.PrefixActor))+"/labels/env",
			strings.NewReader(`{"value":"prod"}`))
		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("annotation kind allows arbitrary value", func(t *testing.T) {
		f := newAdapterFixture(t, Annotation)
		id := apid.New(apid.PrefixActor)
		f.store.put(newFake(id))

		// Annotation values are unrestricted; this would fail the label
		// regex but must succeed for annotations.
		w := f.do(http.MethodPut, "/fakes/"+string(id)+"/annotations/note",
			strings.NewReader(`{"value":"any value with spaces and !!!"}`))
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("propagates Conflict from Put closure", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		s := newStore()
		id := apid.New(apid.PrefixActor)
		s.put(newFake(id))

		a := Adapter[apid.ID]{
			Kind:         Label,
			ResourceName: "fake",
			PathPrefix:   "/fakes/:id",
			AuthGet:      permissiveAuth(),
			AuthMutate:   permissiveAuth(),
			ParseID:      parseFakeID,
			Get: func(_ context.Context, id apid.ID) (Resource, error) {
				r, err := s.get(id)
				if err != nil {
					return nil, err
				}
				return r, nil
			},
			Put: func(_ context.Context, _ apid.ID, _ map[string]string) (Resource, error) {
				return nil, httperr.Conflictf("draft locked")
			},
			Delete: func(_ context.Context, _ apid.ID, _ []string) (Resource, error) {
				return nil, errors.New("unused")
			},
		}
		engine := apgin.ForTest(nil)
		a.Register(engine)

		w := httptest.NewRecorder()
		engine.ServeHTTP(w, httptest.NewRequest(http.MethodPut,
			"/fakes/"+string(id)+"/labels/env",
			strings.NewReader(`{"value":"prod"}`)))
		require.Equal(t, http.StatusConflict, w.Code)
		assert.Contains(t, w.Body.String(), "draft locked")
	})
}

func TestAdapter_HandleDelete(t *testing.T) {
	t.Run("removes key and returns 204", func(t *testing.T) {
		f := newAdapterFixture(t, Label)
		id := apid.New(apid.PrefixActor)
		r := newFake(id)
		r.labels["env"] = "prod"
		f.store.put(r)

		w := f.do(http.MethodDelete, "/fakes/"+string(id)+"/labels/env", nil)
		require.Equal(t, http.StatusNoContent, w.Code)

		stored, _ := f.store.get(id)
		_, exists := stored.labels["env"]
		assert.False(t, exists)
	})

	t.Run("idempotent: 204 when resource missing", func(t *testing.T) {
		f := newAdapterFixture(t, Label)
		w := f.do(http.MethodDelete, "/fakes/"+string(apid.New(apid.PrefixActor))+"/labels/env", nil)
		require.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("204 when key absent on existing resource", func(t *testing.T) {
		f := newAdapterFixture(t, Label)
		id := apid.New(apid.PrefixActor)
		f.store.put(newFake(id))

		w := f.do(http.MethodDelete, "/fakes/"+string(id)+"/labels/never-set", nil)
		require.Equal(t, http.StatusNoContent, w.Code)
	})
}
