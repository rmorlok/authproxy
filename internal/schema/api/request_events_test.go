package api

import (
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
)

func TestNewListRequestEventsResponseJson(t *testing.T) {
	total := int64(12)
	response := NewListRequestEventsResponseJson(nil, "next", &total)
	require.Equal(t, meta.NewTypeMeta("RequestEventList"), response.TypeMeta)
	require.Empty(t, response.Items)
	require.Equal(t, "next", response.Metadata.Continue)
	require.Equal(t, total, *response.Metadata.Total)

	data, err := json.Marshal(response)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"apiVersion":"authproxy.net/v1alpha1",
		"kind":"RequestEventList",
		"metadata":{"continue":"next","total":12},
		"items":[]
	}`, string(data))
}
