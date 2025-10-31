package common

import (
	"encoding/json"
	"fmt"

	"errors"

	"gopkg.in/yaml.v3"
)

type ImageType interface {
	Clone() ImageType
	GetUrl() string
}

type Image struct {
	// Exposed publicly for testing purposes, do not set directly
	InnerVal ImageType `json:"-" yaml:"-"`
}

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

func (d *Image) MarshalJSON() ([]byte, error) {
	if d.InnerVal == nil {
		return json.Marshal(nil)
	}

	return json.Marshal(d.InnerVal)
}

func (d *Image) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		d.InnerVal = nil
		return nil
	}

	if len(data) >= 2 && data[0] == '"' && data[len(data)-1] == '"' {
		// Extract the string without quotes and process it
		content := string(data[1 : len(data)-1])
		d.InnerVal = &ImagePublicUrl{PublicUrl: content, IsDirectString: true}

		return nil
	}

	var tmp map[string]interface{}
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	if _, ok := tmp["public_url"]; ok {
		d.InnerVal = &ImagePublicUrl{}
		return json.Unmarshal(data, d.InnerVal)
	}

	if _, ok := tmp["base64"]; ok {
		d.InnerVal = &ImageBase64{}
		return json.Unmarshal(data, d.InnerVal)
	}

	return errors.New("invalid structure for image type; does not match base64 or public_url")
}

func (i *Image) GetUrl() string {
	if i.InnerVal == nil {
		return ""
	}

	return i.InnerVal.GetUrl()
}

func (i *Image) CloneImage() *Image {
	if i == nil {
		return nil
	}

	return &Image{
		InnerVal: i.InnerVal.Clone(),
	}
}

func (i *Image) Clone() ImageType {
	return i.CloneImage()
}

func (i *Image) Inner() ImageType {
	return i.InnerVal
}

func NewPublicUrlImage(inner ImagePublicUrl) *Image {
	return &Image{
		InnerVal: &inner,
	}
}

func NewBase64Image(inner ImageBase64) *Image {
	return &Image{
		InnerVal: &inner,
	}
}

type ImagePublicUrl struct {
	// PublicUrl is the URL of the image
	PublicUrl string `json:"public_url" yaml:"public_url"`

	// IsDirectString implied how this value was loaded from the config. If true, implies this was loaded
	// as a string value instead of an object with the `public_url` key. This drives how we render to JSON/YAML
	// to be consistent on the round trip.
	//
	// This field is exposed publicly to allow for testing, but should not be manipulated directly.
	IsDirectString bool `json:"-" yaml:"-"`
}

// MarshalJSON provides custom serialization of the object to account for if this was an inline-string or
// a nested object.
func (d ImagePublicUrl) MarshalJSON() ([]byte, error) {
	if d.IsDirectString {
		return []byte(fmt.Sprintf("\"%s\"", d.PublicUrl)), nil
	}

	// Avoid recursive calls to this method
	type Alias ImagePublicUrl

	return json.Marshal(Alias(d))
}

// MarshalYAML provides custom serialization of the object to account for if this was an inline-string or
// a nested object.
func (d ImagePublicUrl) MarshalYAML() (interface{}, error) {
	if d.IsDirectString {
		return d.PublicUrl, nil
	}

	return map[string]string{
		"public_url": d.PublicUrl,
	}, nil
}

func (i *ImagePublicUrl) GetUrl() string {
	return i.PublicUrl
}

func (i *ImagePublicUrl) Clone() ImageType {
	if i == nil {
		return nil
	}

	clone := *i
	return &clone
}

type ImageBase64 struct {
	MimeType string `json:"mime_type" yaml:"mime_type"`
	Base64   string `json:"base64" yaml:"base64"`
}

func (i *ImageBase64) GetUrl() string {
	return fmt.Sprintf("data:%s;base64,%s", i.MimeType, i.Base64)
}

func (i *ImageBase64) Clone() ImageType {
	if i == nil {
		return nil
	}

	clone := *i
	return &clone
}

var _ ImageType = &Image{}
var _ ImageType = &ImagePublicUrl{}
var _ ImageType = &ImageBase64{}
