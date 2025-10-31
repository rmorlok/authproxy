package common

type ImageType interface {
	Clone() ImageType
	GetUrl() string
}

type Image struct {
	// Exposed publicly for testing purposes, do not set directly
	InnerVal ImageType `json:"-" yaml:"-"`
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

var _ ImageType = &Image{}
