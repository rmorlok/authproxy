package routes

import (
	"github.com/rmorlok/authproxy/internal/apid"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
)

func configuredConnectorResource(id apid.ID, version uint64, namespace string, labels map[string]string, definition cschema.ConnectorDefinition) sconfig.Connector {
	return sconfig.Connector{
		TypeMeta: meta.NewTypeMeta(cschema.ConnectorKind),
		Metadata: meta.ObjectMeta{
			ID:         id.String(),
			Namespace:  namespace,
			Generation: version,
			Labels:     labels,
		},
		Spec: cschema.ConnectorSpec{Definition: definition},
	}
}
