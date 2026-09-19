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
			refs, err = References(resource)
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
		type SomeNonResourceType struct {}
		_, err := References(SomeNonResourceType{})
		require.Error(t, err)
	})
}
