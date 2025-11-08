package common

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

func (i *Image) MarshalYAML() (interface{}, error) {
	if i.InnerVal == nil {
		return nil, nil
	}

	return i.InnerVal, nil
}

// UnmarshalYAML handles unmarshalling from YAML while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func (i *Image) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		// The image is being supplied directly as a string, so we can assume it's a public URL
		i.InnerVal = &ImagePublicUrl{PublicUrl: value.Value, IsDirectString: true}
		return nil
	}

	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("image expected a mapping node, got %s", KindToString(value.Kind))
	}

	var inner ImageType

fieldLoop:
	for j := 0; j < len(value.Content); j += 2 {
		keyNode := value.Content[j]

		switch keyNode.Value {
		case "public_url":
			inner = &ImagePublicUrl{}
			break fieldLoop
		case "base64":
			inner = &ImageBase64{}
			break fieldLoop
		}
	}

	if inner == nil {
		return fmt.Errorf("invalid structure for image type; does not match base64 or public_url")
	}

	if err := value.Decode(inner); err != nil {
		return err
	}

	i.InnerVal = inner
	return nil
}
