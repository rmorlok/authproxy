package main

import (
	"encoding/json"
	"testing"

	"github.com/go-resty/resty/v2"
	"github.com/rmorlok/authproxy/internal/apid"
	routes2 "github.com/rmorlok/authproxy/internal/routes"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
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
	resource := routes2.ConnectionJson{
		Id:   apid.MustParse("cxn_test1234567890ab"),
		Name: scommon.ResourceName("production-crm"),
	}

	encoded, err := json.Marshal(resource)
	require.NoError(t, err)
	var projected map[string]any
	require.NoError(t, json.Unmarshal(encoded, &projected))
	require.Equal(t, "cxn_test1234567890ab", projected["id"])
	require.Equal(t, "production-crm", projected["name"])
}
