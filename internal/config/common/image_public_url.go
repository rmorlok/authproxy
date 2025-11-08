package common

import (
	"encoding/json"
	"fmt"
)

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

func NewPublicUrlImage(inner ImagePublicUrl) *Image {
	return &Image{
		InnerVal: &inner,
	}
}

var _ ImageType = &ImagePublicUrl{}
