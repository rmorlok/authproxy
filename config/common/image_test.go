package common

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestImage(t *testing.T) {
	t.Run("round trip starting from objects", func(t *testing.T) {
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
					IsDirectString: false,
				},
			},
			{
				name: "base64",
				Image: &ImageBase64{
					Base64:   "ywAAAAAAQABAAACAUwAOw==",
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
		t.Run("roundtrip", func(t *testing.T) {
			tests := []struct {
				name     string
				data     string
				expected ImageType
			}{
				{
					name: "inline public url",
					expected: &ImagePublicUrl{
						PublicUrl:      "https://example.com/image.png",
						IsDirectString: true,
					},
					data: `https://example.com/image.png`,
				},
				{
					name: "public url",
					expected: &ImagePublicUrl{
						PublicUrl:      "https://example.com/image.png",
						IsDirectString: false,
					},
					data: `
public_url: https://example.com/image.png
`,
				},
				{
					name: "base64",
					expected: &ImageBase64{
						Base64:   "ywAAAAAAQABAAACAUwAOw==",
						MimeType: "image/png",
					},
					data: `
mime_type: image/png
base64: ywAAAAAAQABAAACAUwAOw==
`,
				},
			}

			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					var image Image
					err := yaml.Unmarshal([]byte(test.data), &image)
					require.NoError(t, err)
					require.Equal(t, test.expected, image.Inner())
					require.Equal(t, strings.TrimSpace(test.data), strings.TrimSpace(MustMarshalToYamlString(image.Inner())))
				})
			}
		})
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
	t.Run("json", func(t *testing.T) {
		t.Run("roundtrip", func(t *testing.T) {
			tests := []struct {
				name     string
				data     string
				expected ImageType
			}{
				{
					name: "inline public url",
					expected: &ImagePublicUrl{
						PublicUrl:      "https://example.com/image.png",
						IsDirectString: true,
					},
					data: `"https://example.com/image.png"`,
				},
				{
					name: "public url",
					expected: &ImagePublicUrl{
						PublicUrl:      "https://example.com/image.png",
						IsDirectString: false,
					},
					data: `{"public_url":"https://example.com/image.png"}`,
				},
				{
					name: "base64",
					expected: &ImageBase64{
						Base64:   "ywAAAAAAQABAAACAUwAOw==",
						MimeType: "image/png",
					},
					data: `{"mime_type":"image/png","base64":"ywAAAAAAQABAAACAUwAOw=="}`,
				},
			}

			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					var image Image
					err := json.Unmarshal([]byte(test.data), &image)
					require.NoError(t, err)
					require.Equal(t, test.expected, image.Inner())
					require.Equal(t, strings.TrimSpace(test.data), strings.TrimSpace(MustMarshalToJsonString(image.Inner())))
				})
			}
		})
	})
}
