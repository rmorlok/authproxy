package common

import (
	"fmt"
	"strconv"

	"gopkg.in/yaml.v3"
)

func (sv *IntegerValue) MarshalYAML() (interface{}, error) {
	if sv.InnerVal == nil {
		return nil, nil
	}

	// Serialize directly as an integer if that's how it was loaded
	if v, ok := sv.InnerVal.(*IntegerValueDirect); ok && v.IsDirect {
		return v.Value, nil
	}

	return sv.InnerVal, nil
}

// UnmarshalYAML handles unmarshalling from YAML while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func (sv *IntegerValue) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		val, err := strconv.ParseInt(value.Value, 10, 64)
		if err != nil {
			return err
		}

		sv.InnerVal = &IntegerValueDirect{
			Value:    val,
			IsDirect: true,
		}
		return nil
	}

	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("integer value expected a scalar or mapping node, got %s", KindToString(value.Kind))
	}

	var keyData IntegerValueType

fieldLoop:
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]

		switch keyNode.Value {
		case "value":
			keyData = &IntegerValueDirect{}
			break fieldLoop
		case "env_var":
			keyData = &IntegerValueEnvVar{}
			break fieldLoop
		}
	}

	if keyData == nil {
		return fmt.Errorf("invalid structure for value type; does not match value, value attribute, env_var")
	}

	if err := value.Decode(keyData); err != nil {
		return err
	}

	sv.InnerVal = keyData
	return nil
}
