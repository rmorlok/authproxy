package connectors

import (
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
)

func testConfiguredConnector(id apid.ID, namespace, name string, version uint64) Connector {
	connector := Connector{
		TypeMeta: meta.NewTypeMeta(ConnectorKind),
		Metadata: meta.ObjectMeta{
			ID:         id.String(),
			Name:       common.ResourceName(name),
			Namespace:  namespace,
			Generation: version,
			Labels:     map[string]string{"type": "shared-label-value"},
		},
		Spec: ConnectorSpec{Definition: ConnectorDefinition{
			DisplayName: "Test Connector",
			Description: "Test Description",
		}},
	}
	return connector
}

func TestConnectorsValidateUsesNamesForIdentity(t *testing.T) {
	id1 := apid.MustParse("cxr_test1111111111aa")
	id2 := apid.MustParse("cxr_test2222222222aa")

	tests := []struct {
		name       string
		connectors []Connector
		errMessage string
	}{
		{
			name: "name only",
			connectors: []Connector{
				testConfiguredConnector(apid.Nil, "root", "first", 0),
			},
		},
		{
			name: "id only defaults name later",
			connectors: []Connector{
				testConfiguredConnector(id1, "root", "", 0),
			},
		},
		{
			name: "same labels do not identify connectors",
			connectors: []Connector{
				testConfiguredConnector(apid.Nil, "root", "first", 0),
				testConfiguredConnector(apid.Nil, "root", "second", 0),
			},
		},
		{
			name: "same name is valid in different namespaces",
			connectors: []Connector{
				testConfiguredConnector(apid.Nil, "root.first", "shared", 0),
				testConfiguredConnector(apid.Nil, "root.second", "shared", 0),
			},
		},
		{
			name: "versions share a name",
			connectors: []Connector{
				testConfiguredConnector(apid.Nil, "root", "shared", 1),
				testConfiguredConnector(apid.Nil, "root", "shared", 2),
			},
		},
		{
			name: "id versions may specify name once",
			connectors: []Connector{
				testConfiguredConnector(id1, "root", "shared", 1),
				testConfiguredConnector(id1, "root", "", 2),
			},
		},
		{
			name: "missing id and name",
			connectors: []Connector{
				testConfiguredConnector(apid.Nil, "root", "", 0),
			},
			errMessage: "must specify name when id is omitted",
		},
		{
			name: "invalid name",
			connectors: []Connector{
				testConfiguredConnector(apid.Nil, "root", "not valid", 0),
			},
			errMessage: "resource name must start and end",
		},
		{
			name: "duplicate unversioned name",
			connectors: []Connector{
				testConfiguredConnector(apid.Nil, "root", "shared", 0),
				testConfiguredConnector(apid.Nil, "root", "shared", 0),
			},
			errMessage: "multiple unversioned entries",
		},
		{
			name: "duplicate name version",
			connectors: []Connector{
				testConfiguredConnector(apid.Nil, "root", "shared", 1),
				testConfiguredConnector(apid.Nil, "root", "shared", 1),
			},
			errMessage: "duplicate connectors exist for name",
		},
		{
			name: "same name assigned different ids",
			connectors: []Connector{
				testConfiguredConnector(id1, "root", "shared", 1),
				testConfiguredConnector(id2, "root", "shared", 2),
			},
			errMessage: "assigned to multiple ids",
		},
		{
			name: "same name mixes id and name lookup",
			connectors: []Connector{
				testConfiguredConnector(id1, "root", "shared", 1),
				testConfiguredConnector(apid.Nil, "root", "shared", 2),
			},
			errMessage: "mixes entries with and without ids",
		},
		{
			name: "same id assigned multiple names",
			connectors: []Connector{
				testConfiguredConnector(id1, "root", "first", 1),
				testConfiguredConnector(id1, "root", "second", 2),
			},
			errMessage: "assigned multiple names",
		},
		{
			name: "same id assigned multiple namespaces",
			connectors: []Connector{
				testConfiguredConnector(id1, "root.first", "shared", 1),
				testConfiguredConnector(id1, "root.second", "shared", 2),
			},
			errMessage: "assigned to multiple namespaces",
		},
		{
			name: "duplicate id version",
			connectors: []Connector{
				testConfiguredConnector(id1, "root", "shared", 1),
				testConfiguredConnector(id1, "root", "shared", 1),
			},
			errMessage: "duplicate connectors exist for id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := FromList(tt.connectors).Validate(&common.ValidationContext{})
			if tt.errMessage == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.errMessage)
		})
	}
}
