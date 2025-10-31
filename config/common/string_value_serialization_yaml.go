package common

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

func (sv *StringValue) MarshalYAML() (interface{}, error) {
	if sv.InnerVal == nil {
		return nil, nil
	}

	// Serialize directly as a string if that's how it was loaded
	if v, ok := sv.InnerVal.(*StringValueDirect); ok && v.IsDirectString {
		return v.Value, nil
	}

	return sv.InnerVal, nil
}

// UnmarshalYAML handles unmarshalling from YAML while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func (sv *StringValue) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		sv.InnerVal = &StringValueDirect{Value: value.Value, IsDirectString: true}
		return nil
	}

	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("string value expected a scalar or mapping node, got %s", KindToString(value.Kind))
	}

	var keyData StringValueType

fieldLoop:
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]

		switch keyNode.Value {
		case "value":
			keyData = &StringValueDirect{}
			break fieldLoop
		case "base64":
			keyData = &StringValueBase64{}
			break fieldLoop
		case "env_var":
			keyData = &StringValueEnvVar{}
			break fieldLoop
		case "env_var_base64":
			keyData = &StringValueEnvVarBase64{}
			break fieldLoop
		case "path":
			keyData = &StringValueFile{}
			break fieldLoop
		}
	}

	if keyData == nil {
		return fmt.Errorf("invalid structure for value type; does not match value, base64, env_var, file")
	}

	if err := value.Decode(keyData); err != nil {
		return err
	}

	sv.InnerVal = keyData
	return nil
}
