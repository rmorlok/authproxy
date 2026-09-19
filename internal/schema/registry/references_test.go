package registry

import (
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/resources/connection"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
	"github.com/stretchr/testify/require"
)

func TestReferencesDiscoverTypedResourceFields(t *testing.T) {
	t.Run("pointer", func(t *testing.T) {
		t.Run("it finds a pointer reference", func(t *testing.T) {
			ref := meta.ObjectReference{
				APIVersion: meta.APIVersionV1Alpha1,
				Kind:       "Key",
				Name:       "key",
				Namespace:  "root",
			}
			resource := namespace.NewNamespace()
			resource.Spec.EncryptionKeyRef = &ref
			refs, err := References(resource)
			require.NoError(t, err)
			require.Equal(t, []meta.ObjectReference{ref}, refs)
		})

		t.Run("it doesn't return a reference for a nil pointer", func(t *testing.T) {
			resource := namespace.NewNamespace()
			resource.Spec.EncryptionKeyRef = nil
			refs, err := References(resource)
			require.NoError(t, err)
			require.Empty(t, refs)
		})

		t.Run("it finds a nested pointer reference", func(t *testing.T) {
			ref := meta.ObjectReference{
				APIVersion: meta.APIVersionV1Alpha1,
				Kind:       "Connection",
				Name:       "foo",
				Namespace:  "root",
			}
			limit := rate_limit.NewRateLimit()
			limit.Spec.Scope = &rate_limit.RateLimitScope{ConnectionRef: &ref}
			refs, err := References(limit)
			require.NoError(t, err)
			require.Equal(t, []meta.ObjectReference{ref}, refs)
		})
	})

	t.Run("slice", func(t *testing.T) {
		type MapReference struct {
			References []meta.ObjectReference
		}

		// don't validate resource type
		orig := referencesResourceTypeValidator
		referencesResourceTypeValidator = func(a any) error { return nil }
		defer func() {
			referencesResourceTypeValidator = orig
		}()

		t.Run("it finds a reference", func(t *testing.T) {
			ref := meta.ObjectReference{
				APIVersion: meta.APIVersionV1Alpha1,
				Kind:       "Key",
				Name:       "key",
				Namespace:  "root",
			}
			resource := &MapReference{
				References: []meta.ObjectReference{ref},
			}
			refs, err := References(resource)
			require.NoError(t, err)
			require.Equal(t, []meta.ObjectReference{ref}, refs)
		})

		t.Run("it handles empty slice", func(t *testing.T) {
			resource := &MapReference{
				References: []meta.ObjectReference{},
			}
			refs, err := References(resource)
			require.NoError(t, err)
			require.Empty(t, refs)
		})

		t.Run("it handles nil slice", func(t *testing.T) {
			resource := &MapReference{}
			refs, err := References(resource)
			require.NoError(t, err)
			require.Empty(t, refs)
		})
	})

	t.Run("map", func(t *testing.T) {
		type MapReference struct {
			References map[string]meta.ObjectReference
		}

		// don't validate resource type
		orig := referencesResourceTypeValidator
		referencesResourceTypeValidator = func(a any) error { return nil }
		defer func() {
			referencesResourceTypeValidator = orig
		}()

		t.Run("it finds a reference", func(t *testing.T) {
			ref := meta.ObjectReference{
				APIVersion: meta.APIVersionV1Alpha1,
				Kind:       "Key",
				Name:       "key",
				Namespace:  "root",
			}
			resource := &MapReference{
				References: map[string]meta.ObjectReference{"foo": ref},
			}
			refs, err := References(resource)
			require.NoError(t, err)
			require.Equal(t, []meta.ObjectReference{ref}, refs)
		})

		t.Run("it handles empty map", func(t *testing.T) {
			resource := &MapReference{
				References: map[string]meta.ObjectReference{},
			}
			refs, err := References(resource)
			require.NoError(t, err)
			require.Empty(t, refs)
		})

		t.Run("it handles nil map", func(t *testing.T) {
			resource := &MapReference{}
			refs, err := References(resource)
			require.NoError(t, err)
			require.Empty(t, refs)
		})
	})

	t.Run("map pointer", func(t *testing.T) {
		type MapReference struct {
			References map[string]*meta.ObjectReference
		}

		// don't validate resource type
		orig := referencesResourceTypeValidator
		referencesResourceTypeValidator = func(a any) error { return nil }
		defer func() {
			referencesResourceTypeValidator = orig
		}()

		t.Run("it finds a reference", func(t *testing.T) {
			ref := meta.ObjectReference{
				APIVersion: meta.APIVersionV1Alpha1,
				Kind:       "Key",
				Name:       "key",
				Namespace:  "root",
			}
			resource := &MapReference{
				References: map[string]*meta.ObjectReference{"foo": &ref},
			}
			refs, err := References(resource)
			require.NoError(t, err)
			require.Equal(t, []meta.ObjectReference{ref}, refs)
		})

		t.Run("nil pointer", func(t *testing.T) {
			resource := &MapReference{
				References: map[string]*meta.ObjectReference{"foo": nil},
			}
			refs, err := References(resource)
			require.NoError(t, err)
			require.Empty(t, refs)
		})

		t.Run("it handles empty map", func(t *testing.T) {
			resource := &MapReference{
				References: map[string]*meta.ObjectReference{},
			}
			refs, err := References(resource)
			require.NoError(t, err)
			require.Empty(t, refs)
		})

		t.Run("it handles nil map", func(t *testing.T) {
			resource := &MapReference{}
			refs, err := References(resource)
			require.NoError(t, err)
			require.Empty(t, refs)
		})
	})

	t.Run("synthetic resource shapes", func(t *testing.T) {
		// These shapes exercise traversal paths that canonical resources may gain
		// in the future. Keep the override scoped here so resource gating is still
		// exercised by the other tests. Do not run these subtests in parallel.
		orig := referencesResourceTypeValidator
		referencesResourceTypeValidator = func(any) error { return nil }
		t.Cleanup(func() { referencesResourceTypeValidator = orig })

		first := meta.ObjectReference{APIVersion: meta.APIVersionV1Alpha1, Kind: "Key", Name: "first", Namespace: "root"}
		second := meta.ObjectReference{APIVersion: meta.APIVersionV1Alpha1, Kind: "Key", Name: "second", Namespace: "root"}
		assertReferences := func(t *testing.T, resource any, expected ...meta.ObjectReference) {
			t.Helper()
			refs, err := References(resource)
			require.NoError(t, err)
			require.Equal(t, expected, refs)
		}

		t.Run("value", func(t *testing.T) {
			t.Run("it finds value fields in declaration order", func(t *testing.T) {
				resource := struct{ First, Second meta.ObjectReference }{first, second}
				assertReferences(t, resource, first, second)
			})
			t.Run("it skips zero references including non-nil pointers", func(t *testing.T) {
				resource := struct {
					Value   meta.ObjectReference
					Pointer *meta.ObjectReference
				}{Pointer: &meta.ObjectReference{}}
				assertReferences(t, resource)
			})
			t.Run("it preserves a nonzero reference for later validation", func(t *testing.T) {
				ref := meta.ObjectReference{Kind: "Key"}
				assertReferences(t, struct{ Ref meta.ObjectReference }{ref}, ref)
			})
		})

		t.Run("polymorphic", func(t *testing.T) {
			type Wrapper struct {
				InnerVal any `json:"-"`
			}
			t.Run("it follows nested InnerVal wrappers despite the skipped JSON tag", func(t *testing.T) {
				resource := Wrapper{InnerVal: &Wrapper{InnerVal: &struct{ Ref meta.ObjectReference }{first}}}
				assertReferences(t, resource, first)
			})
			t.Run("it follows ordinary interface fields", func(t *testing.T) {
				resource := struct{ Value any }{&first}
				assertReferences(t, resource, first)
			})
			t.Run("it handles a nil interface", func(t *testing.T) {
				assertReferences(t, Wrapper{})
			})
			t.Run("it handles a typed nil inside an interface", func(t *testing.T) {
				assertReferences(t, Wrapper{InnerVal: (*meta.ObjectReference)(nil)})
			})
		})

		t.Run("struct visibility", func(t *testing.T) {
			t.Run("it skips JSON-excluded fields and their descendants", func(t *testing.T) {
				resource := struct {
					Hidden  meta.ObjectReference `json:"-"`
					Nested  any                  `json:"-"`
					Visible meta.ObjectReference `json:"visible,omitempty"`
				}{Hidden: first, Nested: &struct{ Ref meta.ObjectReference }{first}, Visible: second}
				assertReferences(t, resource, second)
			})
			t.Run("it skips unexported fields even with JSON tags", func(t *testing.T) {
				resource := struct {
					hidden  meta.ObjectReference `json:"hidden"`
					Visible meta.ObjectReference
				}{hidden: first, Visible: second}
				assertReferences(t, &resource, second)
			})
			t.Run("it follows exported embedded structs", func(t *testing.T) {
				type Embedded struct{ Ref meta.ObjectReference }
				assertReferences(t, struct{ Embedded }{Embedded{first}}, first)
			})
		})

		t.Run("containers", func(t *testing.T) {
			t.Run("it preserves slice order and skips nil entries", func(t *testing.T) {
				assertReferences(t, []*meta.ObjectReference{&second, nil, &first}, second, first)
			})
			t.Run("it preserves array order and skips zero entries", func(t *testing.T) {
				assertReferences(t, [3]meta.ObjectReference{second, {}, first}, second, first)
			})
			t.Run("it handles an empty array", func(t *testing.T) {
				assertReferences(t, [0]meta.ObjectReference{})
			})
			t.Run("it sorts map keys rather than reference names", func(t *testing.T) {
				resource := map[string]meta.ObjectReference{"z": first, "a": second}
				for i := 0; i < 20; i++ {
					assertReferences(t, resource, second, first)
				}
			})
			t.Run("it traverses mixed nested containers", func(t *testing.T) {
				resource := map[string]any{"refs": []any{[1]*meta.ObjectReference{&first}, map[string]any{"ref": second}}}
				assertReferences(t, resource, first, second)
			})
			t.Run("it does not infer references from untyped JSON objects", func(t *testing.T) {
				resource := map[string]any{"apiVersion": "authproxy/v1alpha1", "kind": "Key", "metadata": map[string]any{"name": "first", "namespace": "root"}}
				assertReferences(t, resource)
			})
		})

		t.Run("cycles and shared values", func(t *testing.T) {
			t.Run("it terminates a pointer cycle and visits sibling fields", func(t *testing.T) {
				type Node struct {
					Next *Node
					Ref  meta.ObjectReference
				}
				resource := &Node{Ref: first}
				resource.Next = resource
				assertReferences(t, resource, first)
			})
			t.Run("it terminates a map cycle and visits remaining entries", func(t *testing.T) {
				resource := map[string]any{"z": first}
				resource["a"] = resource
				assertReferences(t, resource, first)
			})
			t.Run("it terminates a slice cycle and visits remaining elements", func(t *testing.T) {
				resource := make([]any, 2)
				resource[0], resource[1] = resource, first
				assertReferences(t, resource, first)
			})
			t.Run("it visits a shared pointer through each independent path", func(t *testing.T) {
				assertReferences(t, []*meta.ObjectReference{&first, &first}, first, first)
			})
			t.Run("it visits a shared map through each independent path", func(t *testing.T) {
				shared := map[string]any{"ref": first}
				assertReferences(t, []any{shared, shared}, first, first)
			})
			t.Run("it visits a shared slice through each independent path", func(t *testing.T) {
				shared := []meta.ObjectReference{first}
				assertReferences(t, []any{shared, shared}, first, first)
			})
		})

		t.Run("empty roots", func(t *testing.T) {
			t.Run("it handles a nil root", func(t *testing.T) { assertReferences(t, nil) })
			t.Run("it handles a typed nil root", func(t *testing.T) { assertReferences(t, (*meta.ObjectReference)(nil)) })
			t.Run("it ignores scalar values", func(t *testing.T) { assertReferences(t, []any{"text", true, 1}) })
		})
	})

	t.Run("it decouples the references from the underlying data", func(t *testing.T) {
		ref := meta.ObjectReference{
			APIVersion: meta.APIVersionV1Alpha1,
			Kind:       "Key",
			Name:       "key",
			Namespace:  "root",
		}
		resource := namespace.NewNamespace()
		resource.Spec.EncryptionKeyRef = &ref
		refs, err := References(resource)
		require.NoError(t, err)
		require.Equal(t, []meta.ObjectReference{ref}, refs)

		// Change data on the returned reference
		refs[0].Name = "changed"
		require.Equal(t, ref, *resource.Spec.EncryptionKeyRef)
	})

	t.Run("it doesn't return a reference for a resource that doesn't a have a reference", func(t *testing.T) {
		refs, err := References(connection.NewConnection())
		require.NoError(t, err)
		require.Empty(t, refs)
	})

	t.Run("it rejects non-resource types", func(t *testing.T) {
		type SomeNonResourceType struct{}
		_, err := References(SomeNonResourceType{})
		require.Error(t, err)
	})
}
