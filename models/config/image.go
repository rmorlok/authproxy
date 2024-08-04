package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
)

type Image interface {
	GetUrl() string
}

// imageUnmarshalYAML handles unmarshalling from YAML while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func imageUnmarshalYAML(value *yaml.Node) (Image, error) {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected a mapping node, got %v", value.Kind)
	}

	var image Image

fieldLoop:
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]

		switch keyNode.Value {
		case "public_url":
			image = &ImagePublicUrl{}
			break fieldLoop
		case "base64":
			image = &ImageBase64{}
			break fieldLoop
		}
	}

	if image == nil {
		return nil, fmt.Errorf("invalid structure for image type; does not match base64 or public_url")
	}

	if err := value.Decode(image); err != nil {
		return nil, err
	}

	return image, nil
}

type ImagePublicUrl struct {
	PublicUrl string `json:"public_url" yaml:"public_url"`
}

func (i *ImagePublicUrl) GetUrl() string {
	return i.PublicUrl
}

type ImageBase64 struct {
	MimeType string `json:"mime_type" yaml:"mime_type"`
	Base64   string `json:"base64" yaml:"base64"`
}

func (i *ImageBase64) GetUrl() string {
	return fmt.Sprintf("data:%s;base64,%s", i.MimeType, i.Base64)
}
