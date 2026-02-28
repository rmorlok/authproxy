package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

func (b *BlobStorage) MarshalYAML() (interface{}, error) {
	if b.InnerVal == nil {
		return nil, nil
	}
	return b.InnerVal, nil
}

// UnmarshalYAML handles unmarshalling from YAML while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func (b *BlobStorage) UnmarshalYAML(value *yaml.Node) error {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("BlobStorage expected a mapping node, got %s", KindToString(value.Kind))
	}

	var bs BlobStorageImpl = &BlobStorageS3{}

fieldLoop:
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valueNode := value.Content[i+1]

		switch keyNode.Value {
		case "provider":
			switch BlobStorageProvider(valueNode.Value) {
			case BlobStorageProviderMemory:
				bs = &BlobStorageMemory{
					Provider: BlobStorageProviderMemory,
				}
				break fieldLoop
			case BlobStorageProviderS3:
				bs = &BlobStorageS3{
					Provider: BlobStorageProviderS3,
				}
				break fieldLoop
			default:
				return fmt.Errorf("unknown blob storage provider %v", valueNode.Value)
			}
		}
	}

	if err := value.Decode(bs); err != nil {
		return err
	}

	b.InnerVal = bs
	return nil
}
