package main

import (
	"encoding/json"
	"testing"

	"github.com/go-resty/resty/v2"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	connectionschema "github.com/rmorlok/authproxy/internal/schema/resources/connection"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
)

func TestResourceListCommandsExposeExactNameFilter(t *testing.T) {
	connectionCommand := cmdListConnections()
	require.NotNil(t, connectionCommand.Flags().Lookup("name"))
	connectorCommand := cmdListConnectors()
	require.NotNil(t, connectorCommand.Flags().Lookup("name"))

	connectionRequest := resty.New().R()
	setConnectionListQuery(connectionRequest, "production-crm", "configured", "updated_at desc", "cursor-1")
	require.Equal(t, "production-crm", connectionRequest.QueryParam.Get("name"))
	require.Equal(t, "configured", connectionRequest.QueryParam.Get("state"))
	require.Equal(t, "cursor-1", connectionRequest.QueryParam.Get("cursor"))

	connectorRequest := resty.New().R()
	setConnectorListQuery(connectorRequest, "salesforce", "primary", "crm", "name asc", "cursor-2")
	require.Equal(t, "salesforce", connectorRequest.QueryParam.Get("name"))
	require.Equal(t, "crm", connectorRequest.QueryParam.Get("type"))
	require.Equal(t, "cursor-2", connectorRequest.QueryParam.Get("cursor"))
}

func TestResourceListJSONKeepsNameAndImmutableID(t *testing.T) {
	resource := connectionschema.Connection{
		TypeMeta: meta.NewTypeMeta(connectionschema.ConnectionKind),
		Metadata: meta.ObjectMeta{
			ID:   "cxn_test1234567890ab",
			Name: scommon.ResourceName("production-crm"),
		},
	}

	encoded, err := json.Marshal(resource)
	require.NoError(t, err)
	var projected map[string]any
	require.NoError(t, json.Unmarshal(encoded, &projected))
	metadata := projected["metadata"].(map[string]any)
	require.Equal(t, "cxn_test1234567890ab", metadata["id"])
	require.Equal(t, "production-crm", metadata["name"])
}
