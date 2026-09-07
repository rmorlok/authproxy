package api

import (
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
)

func TestNewDataSourceOptionListUsesCanonicalEnvelope(t *testing.T) {
	list := NewDataSourceOptionList([]DataSourceOptionJson{{Value: "ws-123", Label: "Workspace"}})

	require.Equal(t, meta.APIVersionV1Alpha1, list.APIVersion)
	require.Equal(t, meta.Kind("DataSourceOptionList"), list.Kind)
	require.Equal(t, []DataSourceOptionJson{{Value: "ws-123", Label: "Workspace"}}, list.Items)
}

func TestNewConnectionScopeListMergesRequestedAndGrantedScopes(t *testing.T) {
	list := NewConnectionScopeList(
		[]string{"read", "write", "read"},
		[]string{"read", "provider-added"},
	)

	require.Equal(t, meta.APIVersionV1Alpha1, list.APIVersion)
	require.Equal(t, meta.Kind("ConnectionScopeList"), list.Kind)
	require.Equal(t, []ConnectionScopeJson{
		{Name: "read", Requested: true, Granted: true},
		{Name: "write", Requested: true, Granted: false},
		{Name: "provider-added", Requested: false, Granted: true},
	}, list.Items)
}

func TestProjectionListsNormalizeNilItems(t *testing.T) {
	require.NotNil(t, NewDataSourceOptionList(nil).Items)
	require.NotNil(t, NewConnectionScopeList(nil, nil).Items)
}
