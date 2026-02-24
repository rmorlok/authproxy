package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

func (c *AwsCredentials) MarshalYAML() (interface{}, error) {
	if c.InnerVal == nil {
		return nil, nil
	}
	return c.InnerVal, nil
}

// UnmarshalYAML handles unmarshalling from YAML while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func (c *AwsCredentials) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("AwsCredentials expected a mapping node, got %s", KindToString(value.Kind))
	}

	var creds AwsCredentialsImpl = &AwsCredentialsImplicit{}

fieldLoop:
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valueNode := value.Content[i+1]

		switch keyNode.Value {
		case "type":
			switch AwsCredentialsType(valueNode.Value) {
			case AwsCredentialsTypeAccessKey:
				creds = &AwsCredentialsAccessKey{
					Type: AwsCredentialsTypeAccessKey,
				}
				break fieldLoop
			case AwsCredentialsTypeImplicit:
				creds = &AwsCredentialsImplicit{
					Type: AwsCredentialsTypeImplicit,
				}
				break fieldLoop
			default:
				return fmt.Errorf("unknown blob storage credentials type %v", valueNode.Value)
			}
		}
	}

	if err := value.Decode(creds); err != nil {
		return err
	}

	c.InnerVal = creds
	return nil
}
