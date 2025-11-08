package config

import (
	"context"
	"crypto/tls"
	"fmt"
	"gopkg.in/yaml.v3"
	"log"
)

type TlsConfigVals struct {
	Cert KeyData `json:"cert" yaml:"cert"`
	Key  KeyData `json:"key" yaml:"key"`
}

func (tcv *TlsConfigVals) UnmarshalYAML(value *yaml.Node) error {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("tls config vals expected a mapping node, got %s", KindToString(value.Kind))
	}

	var cert KeyData
	var key KeyData

	// Handle custom unmarshalling for some attributes. Iterate through the mapping node's content,
	// which will be sequences of keys, then values.
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valueNode := value.Content[i+1]

		var err error
		matched := false

		switch keyNode.Value {
		case "cert":
			if cert, err = keyDataUnmarshalYAML(valueNode); err != nil {
				return err
			}
			matched = true
		case "key":
			if key, err = keyDataUnmarshalYAML(valueNode); err != nil {
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
	type RawType TlsConfigVals
	raw := (*RawType)(tcv)
	if err := value.Decode(raw); err != nil {
		return err
	}

	// Set the custom unmarshalled types
	raw.Cert = cert
	raw.Key = key

	return nil
}

func (tcv *TlsConfigVals) TlsConfig(ctx context.Context, s HttpServiceLike) (*tls.Config, error) {
	if tcv == nil {
		return nil, nil
	}

	if tcv.Cert == nil || tcv.Key == nil {
		return nil, fmt.Errorf("tls config vals must have cert and key")
	}

	if !tcv.Cert.HasData(ctx) || !tcv.Key.HasData(ctx) {
		return nil, fmt.Errorf("tls config vals must have cert and key data")
	}

	cert, err := tcv.Cert.GetData(ctx)
	if err != nil {
		return nil, err
	}

	key, err := tcv.Key.GetData(ctx)
	if err != nil {
		return nil, err
	}

	// Create certificate from byte slices
	kp, err := tls.X509KeyPair(cert, key)
	if err != nil {
		log.Fatalf("Failed to load key pair: %v", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{kp},
	}, nil
}

var _ TlsConfig = (*TlsConfigVals)(nil)
