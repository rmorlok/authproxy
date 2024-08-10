package config

type KeyPublicPrivate struct {
	PublicKey  string `json:"public_key" yaml:"public_key"`
	PrivateKey string `json:"private_key" yaml:"private_key"`
}

// keyPublicPrivateUnmarshalYAML handles unmarshalling from YAML while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
/*func keyPublicPrivateUnmarshalYAML(value *yaml.Node) (Key, error) {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected a mapping node, got %v", value.Kind)
	}

	var publicKeyData KeyData
	var privateKeyData KeyData
	var err error

	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valueNode := value.Content[i+1]

		matched := false

		switch keyNode.Value {
		case "public_key":
			publicKeyData, err = keyDataUnmarshalYAML(valueNode)
			if err != nil {
				return nil, err
			}
			matched = true
		case "private_key":
			privateKeyData, err = keyDataUnmarshalYAML(valueNode)
			if err != nil {
				return nil, err
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

	if publicKeyData == nil && privateKeyData == nil {
		return nil, fmt.Errorf("invalid structure for public private key; must have at least public or private key data")
	}

	// Let the rest unmarshall normally
	type RawType KeyPublicPrivate
	raw := (*RawType)(i)
	if err := value.Decode(raw); err != nil {
		return err
	}

	return keyData, nil
}*/

func (kpp *KeyPublicPrivate) CanSign() bool {
	return false
}

func (kpp *KeyPublicPrivate) CanVerifySignature() bool {
	return false
}
