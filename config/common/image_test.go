package common

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestImage(t *testing.T) {
	assert := require.New(t)

	t.Run("yaml parse", func(t *testing.T) {
		t.Run("public url", func(t *testing.T) {
			data := `
public_url: https://upload.wikimedia.org/wikipedia/commons/thumb/1/12/Google_Drive_icon_%282020%29.svg/1024px-Google_Drive_icon_%282020%29.svg.png?20221103153031
`
			image, err := UnmarshallYamlImageString(data)
			assert.NoError(err)
			assert.Equal(&ImagePublicUrl{
				PublicUrl: "https://upload.wikimedia.org/wikipedia/commons/thumb/1/12/Google_Drive_icon_%282020%29.svg/1024px-Google_Drive_icon_%282020%29.svg.png?20221103153031",
			}, image)
		})
		t.Run("base64", func(t *testing.T) {
			data := `
mime_type: image/png
base64: iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/wcAAwAB/1J8qkwAAAAASUVORK5CYII=`
			image, err := UnmarshallYamlImageString(data)
			assert.NoError(err)
			assert.Equal(&ImageBase64{
				MimeType: "image/png",
				Base64:   "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/wcAAwAB/1J8qkwAAAAASUVORK5CYII=",
			}, image)
		})
	})

	t.Run("yaml gen", func(t *testing.T) {
		t.Run("public url", func(t *testing.T) {
			data := &ImagePublicUrl{
				PublicUrl: "https://upload.wikimedia.org/wikipedia/commons/thumb/1/12/Google_Drive_icon_%282020%29.svg/1024px-Google_Drive_icon_%282020%29.svg.png?20221103153031",
			}
			assert.Equal("public_url: https://upload.wikimedia.org/wikipedia/commons/thumb/1/12/Google_Drive_icon_%282020%29.svg/1024px-Google_Drive_icon_%282020%29.svg.png?20221103153031\n", MustMarshalToYamlString(data))
		})
		t.Run("base64", func(t *testing.T) {
			data := &ImageBase64{
				MimeType: "image/png",
				Base64:   "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/wcAAwAB/1J8qkwAAAAASUVORK5CYII=",
			}
			assert.Equal(`mime_type: image/png
base64: iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/wcAAwAB/1J8qkwAAAAASUVORK5CYII=
`, MustMarshalToYamlString(data))
		})
	})
}
