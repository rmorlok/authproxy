package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	connectorschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestResourceOutputRetainsV1Alpha1Envelope(t *testing.T) {
	connector := connectorschema.NewConnector()
	connector.Metadata = meta.ObjectMeta{
		ID:         "cxr_testconnector0001",
		Name:       common.ResourceName("test-connector"),
		Namespace:  "root.testing",
		Generation: 3,
	}
	connector.Spec.Definition.DisplayName = "Test connector"
	connector.Status = &connectorschema.ConnectorStatus{
		Release: connectorschema.ConnectorReleaseStatus{State: connectorschema.ConnectorReleaseStateActive},
	}

	var buffer bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buffer)
	out := OutputMultiple[connectorschema.Connector](cmd)
	out.Emit(*connector)
	out.Done()

	var resources []map[string]any
	require.NoError(t, json.Unmarshal(buffer.Bytes(), &resources))
	require.Len(t, resources, 1)
	require.Equal(t, "authproxy.net/v1alpha1", resources[0]["apiVersion"])
	require.Equal(t, "Connector", resources[0]["kind"])
	require.NotContains(t, resources[0], "id")
	require.NotContains(t, resources[0], "name")

	metadata, ok := resources[0]["metadata"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "cxr_testconnector0001", metadata["id"])
	require.Equal(t, "test-connector", metadata["name"])
	require.Equal(t, "root.testing", metadata["namespace"])
	require.Equal(t, float64(3), metadata["generation"])
	require.Contains(t, resources[0], "spec")
	require.Contains(t, resources[0], "status")
}
