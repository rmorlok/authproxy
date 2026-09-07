package key

import (
	"fmt"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util"
	"gopkg.in/yaml.v3"
)

func (k *SigningKey) MarshalYAML() (interface{}, error) {
	if k == nil || k.InnerVal == nil {
		return nil, nil
	}

	return k.InnerVal, nil
}

// UnmarshalYAML handles unmarshalling from YAML while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func (k *SigningKey) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("key expected a mapping node, got %s", common.KindToString(value.Kind))
	}

	var key SigningKeyType

fieldLoop:
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]

		switch keyNode.Value {
		case "publicKey":
			key = &KeyPublicPrivate{}
			break fieldLoop
		case "privateKey":
			key = &KeyPublicPrivate{}
			break fieldLoop
		case "sharedKey":
			key = &KeyShared{}
			break fieldLoop
		}
	}

	if key == nil {
		return fmt.Errorf("invalid structure for key type; does not match value, publicKey/privateKey or sharedKey")
	}

	if err := util.DecodeYAMLNodeStrict(value, key); err != nil {
		return err
	}

	k.InnerVal = key

	return nil
}
