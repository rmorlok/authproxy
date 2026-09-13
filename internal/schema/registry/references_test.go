package registry

import (
	"github.com/rmorlok/authproxy/internal/schema/resources/connection"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestReferencesDiscoverTypedResourceFields(t *testing.T) {
	ref := meta.ObjectReference{APIVersion: meta.APIVersionV1Alpha1, Kind: "Key", Name: "key", Namespace: "root"}
	resource := namespace.NewNamespace()
	resource.Spec.EncryptionKeyRef = &ref
	refs, err := References(resource)
	require.NoError(t, err)
	require.Equal(t, []meta.ObjectReference{ref}, refs)
	refs[0].Name = "changed"
	require.Equal(t, ref, *resource.Spec.EncryptionKeyRef)
	limit := rate_limit.NewRateLimit()
	limit.Spec.Scope = &rate_limit.RateLimitScope{ConnectionRef: &ref}
	refs, err = References(limit)
	require.NoError(t, err)
	require.Equal(t, []meta.ObjectReference{ref}, refs)
	resource.Spec.EncryptionKeyRef = nil
	refs, err = References(resource)
	require.NoError(t, err)
	require.Empty(t, refs)
	refs, err = References(connection.NewConnection())
	require.NoError(t, err)
	require.Empty(t, refs)
	_, err = References(map[string]any{"kind": "Key", "id": "fake"})
	require.Error(t, err)
}
