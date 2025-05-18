package connectors

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	"gopkg.in/yaml.v3"

	"github.com/rmorlok/authproxy/config/common"
)

type Connector struct {
	// Id is the global id for this connector. This does not change version to version. In the config file, this can
	// be omitted if there is only one instance of a particular type of connector. If there are multiple connectors that
	// share the same type, the ids would need to be explicitly defined.
	Id uuid.UUID `json:"id" yaml:"id"`

	// Type is the logical type of the connector. Multiple connectors can implement the same type. This might represent
	// two different ways of connecting to the same 3rd party system (e.g. via API key and OAuth) where the types of auth
	// don't have an impact on the capabilities of the connector.
	Type string `json:"type" yaml:"type"`

	// Version is the logical version of the connector. When auth materially changes, such as adding new scopes,
	// changing client ids/secrets, adding configuration settings, etc. the logical version of the connector must change
	// so that existing connections can be managed through the migration process. If specified explicitly in the config
	// file, this version will prevent changes to the system by preventing startup. If unspecified, the system will
	// automatically create versions based on the configuration changing. If specified explicitly, this version must
	// start with 1 (zero implies unspecified).
	Version     uint64       `json:"version" yaml:"version"`
	DisplayName string       `json:"display_name" yaml:"display_name"`
	Logo        common.Image `json:"logo" yaml:"logo"`
	Description string       `json:"description" yaml:"description"`
	Auth        Auth         `json:"auth" yaml:"auth"`
}

func (c *Connector) UnmarshalYAML(value *yaml.Node) error {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("connector expected a mapping node, got %s", common.KindToString(value.Kind))
	}

	var image common.Image
	var auth Auth

	// Handle custom unmarshalling for some attributes. Iterate through the mapping node's content,
	// which will be sequences of keys, then values.
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valueNode := value.Content[i+1]

		var err error
		matched := false

		switch keyNode.Value {
		case "logo":
			if image, err = common.ImageUnmarshalYAML(valueNode); err != nil {
				return err
			}
			matched = true
		case "auth":
			if auth, err = authUnmarshalYAML(valueNode); err != nil {
				return err
			}
			matched = true
		}

		if matched {
			// Remove the key/value from the raw unmarshalling, and pull back our index
			// because of the changing slice size to the left of what we are indexing
			value.Content = append(value.Content[:i], value.Content[i+2:]...)
			i -= 2
		}
	}

	// Let the rest unmarshall normally
	type RawType Connector
	raw := (*RawType)(c)
	if err := value.Decode(raw); err != nil {
		return err
	}

	// Set the custom unmarshalled types
	raw.Logo = image
	raw.Auth = auth

	return nil
}

func (c *Connector) Validate() error {
	result := &multierror.Error{}

	if c.Type == "" {
		result = multierror.Append(result, fmt.Errorf("connector must have type"))
	}

	return result.ErrorOrNil()
}
