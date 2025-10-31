package common

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestImage(t *testing.T) {
	t.Run("round trip", func(t *testing.T) {
		tests := []struct {
			name  string
			Image ImageType
		}{
			{
				name: "inline public url",
				Image: &ImagePublicUrl{
					PublicUrl:      "https://example.com/image.png",
					IsDirectString: true,
				},
			},
			{
				name: "public url",
				Image: &ImagePublicUrl{
					PublicUrl:      "https://example.com/image.png",
					IsDirectString: true,
				},
			},
			{
				name: "base64",
				Image: &ImageBase64{
					Base64:   "https://example.com/image.png",
					MimeType: "image/png",
				},
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				t.Run("yaml", func(t *testing.T) {
					wrapper := &Image{InnerVal: test.Image}
					data, err := yaml.Marshal(wrapper)
					require.NoError(t, err)

					var output Image

					err = yaml.Unmarshal(data, &output)
					require.NoError(t, err)
					require.Equal(t, test.Image, output.Inner())
				})
				t.Run("json", func(t *testing.T) {
					wrapper := &Image{InnerVal: test.Image}
					data, err := json.Marshal(wrapper)
					require.NoError(t, err)

					var output Image
					err = json.Unmarshal(data, &output)
					require.NoError(t, err)
					require.Equal(t, test.Image, output.Inner())
				})
			})
		}
	})

	t.Run("yaml", func(t *testing.T) {
		t.Run("parse", func(t *testing.T) {
			t.Run("inline public url", func(t *testing.T) {
				data := `https://example.com/image.png
`
				var image Image
				err := yaml.Unmarshal([]byte(data), &image)
				require.NoError(t, err)
				require.Equal(t, &ImagePublicUrl{
					PublicUrl:      "https://example.com/image.png",
					IsDirectString: true,
				}, image.Inner())
			})
			t.Run("public url", func(t *testing.T) {
				data := `
public_url: https://example.com/image.png
`
				var image Image
				err := yaml.Unmarshal([]byte(data), &image)
				require.NoError(t, err)
				require.Equal(t, &ImagePublicUrl{
					PublicUrl: "https://example.com/image.png",
				}, image.Inner())
			})
			t.Run("base64", func(t *testing.T) {
				data := `
mime_type: image/png
base64: iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/wcAAwAB/1J8qkwAAAAASUVORK5CYII=`
				var image Image
				err := yaml.Unmarshal([]byte(data), &image)
				require.NoError(t, err)
				require.Equal(t, &ImageBase64{
					MimeType: "image/png",
					Base64:   "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/wcAAwAB/1J8qkwAAAAASUVORK5CYII=",
				}, image.Inner())
			})
		})

		t.Run("yaml gen", func(t *testing.T) {
			t.Run("inline public url", func(t *testing.T) {
				data := &ImagePublicUrl{
					PublicUrl:      "https://example.com/image.png",
					IsDirectString: true,
				}
				require.Equal(t, "https://example.com/image.png\n", MustMarshalToYamlString(data))
			})
			t.Run("public url", func(t *testing.T) {
				data := &ImagePublicUrl{
					PublicUrl: "https://example.com/image.png",
				}
				require.Equal(t, "public_url: https://example.com/image.png\n", MustMarshalToYamlString(data))
			})
			t.Run("base64", func(t *testing.T) {
				data := &ImageBase64{
					MimeType: "image/png",
					Base64:   "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/wcAAwAB/1J8qkwAAAAASUVORK5CYII=",
				}
				require.Equal(t, `mime_type: image/png
base64: iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/wcAAwAB/1J8qkwAAAAASUVORK5CYII=
`, MustMarshalToYamlString(data))
			})
		})
	})
}
