package common

import "fmt"

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

func NewBase64Image(inner ImageBase64) *Image {
	return &Image{
		InnerVal: &inner,
	}
}

var _ ImageType = &ImageBase64{}
