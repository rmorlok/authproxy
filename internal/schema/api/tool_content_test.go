package api

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// richToolContentForTest covers every variant and independently owned mutable
// field, including empty payloads and provider metadata at both resource levels.
func richToolContentForTest() []ToolContentBlock {
	return []ToolContentBlock{
		{
			Type: ToolContentTypeText,
			Text: util.ToPtr(""),
			Annotations: &ToolContentAnnotations{
				Audience:     []string{"user", "assistant"},
				Priority:     util.ToPtr(0.0),
				LastModified: "provider timestamp",
			},
			Provider: common.RawJSON(`{"example":9007199254740993}`),
		},
		{
			Type:     ToolContentTypeImage,
			Data:     util.ToPtr("AAEC"),
			MIMEType: "image/png",
		},
		{
			Type:     ToolContentTypeAudio,
			Data:     util.ToPtr(""),
			MIMEType: "audio/wav",
		},
		{
			Type:        ToolContentTypeResourceLink,
			URI:         "custom:report#section",
			Name:        util.ToPtr(""),
			Title:       "Report",
			Description: "A provider resource",
			MIMEType:    "application/pdf",
			Size:        util.ToPtr(int64(-1)),
			Icons: []ToolContentIcon{
				{
					Src:      "data:image/png;base64,AAEC",
					MIMEType: "image/png",
					Sizes:    []string{"any"},
					Theme:    "dark",
				},
			},
		},
		{
			Type: ToolContentTypeResource,
			Resource: &ToolResourceContent{
				URI:      "urn:authproxy:invocation:example:text",
				MIMEType: "text/plain; charset=utf-8",
				Text:     util.ToPtr(""),
				Provider: common.RawJSON(`{"tenant_note":true}`),
			},
			Provider: common.RawJSON(`{"outer":false}`),
		},
		{
			Type: ToolContentTypeResource,
			Resource: &ToolResourceContent{
				URI:      "custom:binary",
				Blob:     util.ToPtr(""),
				MIMEType: "application/octet-stream",
			},
		},
	}
}

// TestToolContentRoundTrip preserves ordered rich content, empty strings, exact
// URI identifiers, numeric precision, and rendering hints without network I/O.
func TestToolContentRoundTrip(t *testing.T) {
	for _, block := range richToolContentForTest() {
		require.NoError(t, block.Validate(nil))
		for _, format := range []struct {
			encode func(any) ([]byte, error)
			decode func([]byte, any) error
		}{
			{json.Marshal, util.DecodeJSONStrict},
			{yaml.Marshal, util.DecodeYAMLStrict},
		} {
			encoded, err := format.encode(block)
			require.NoError(t, err)

			var decoded ToolContentBlock
			require.NoError(t, format.decode(encoded, &decoded))
			require.NoError(t, decoded.Validate(nil))
			require.Equal(t, block, decoded)
		}
	}
}

// TestToolContentValidation rejects ambiguous variants and invalid encoded
// data while allowing provider-defined URI schemes and presentation strings.
func TestToolContentValidation(t *testing.T) {
	for _, test := range []struct {
		name  string
		block ToolContentBlock
		field string
	}{
		{"unknown", ToolContentBlock{Type: "video"}, "type"},
		{"absent text", ToolContentBlock{Type: ToolContentTypeText}, "text"},
		{"mixed payload", ToolContentBlock{Type: ToolContentTypeText, Text: util.ToPtr("hello"), Data: util.ToPtr("")}, "another content type"},
		{"text with URI", ToolContentBlock{Type: ToolContentTypeText, Text: util.ToPtr("hello"), URI: "urn:example"}, "another content type"},
		{"missing image data", ToolContentBlock{Type: ToolContentTypeImage, MIMEType: "image/png"}, "data"},
		{"missing media type", ToolContentBlock{Type: ToolContentTypeImage, Data: util.ToPtr("AAEC")}, "mimeType"},
		{"invalid base64", ToolContentBlock{Type: ToolContentTypeAudio, Data: util.ToPtr("invalid!"), MIMEType: "audio/wav"}, "data"},
		{"noncanonical trailing bits", ToolContentBlock{Type: ToolContentTypeImage, Data: util.ToPtr("AB=="), MIMEType: "image/png"}, "data"},
		{"invalid media type", ToolContentBlock{Type: ToolContentTypeImage, Data: util.ToPtr(""), MIMEType: "not-a-type"}, "mimeType"},
		{"missing link name", ToolContentBlock{Type: ToolContentTypeResourceLink, URI: "urn:report"}, "name"},
		{"relative URI", ToolContentBlock{Type: ToolContentTypeResourceLink, URI: "./report", Name: util.ToPtr("Report")}, "uri"},
		{"link with payload", ToolContentBlock{Type: ToolContentTypeResourceLink, URI: "urn:report", Name: util.ToPtr("Report"), Text: util.ToPtr("")}, "another content type"},
		{"missing resource", ToolContentBlock{Type: ToolContentTypeResource}, "resource"},
		{"two representations", ToolContentBlock{Type: ToolContentTypeResource, Resource: &ToolResourceContent{URI: "urn:report", Text: util.ToPtr(""), Blob: util.ToPtr("")}}, "exactly one"},
		{"no representation", ToolContentBlock{Type: ToolContentTypeResource, Resource: &ToolResourceContent{URI: "urn:report"}}, "exactly one"},
		{"invalid blob", ToolContentBlock{Type: ToolContentTypeResource, Resource: &ToolResourceContent{URI: "urn:report", Blob: util.ToPtr("?")}}, "blob"},
		{"nonobject provider", ToolContentBlock{Type: ToolContentTypeText, Text: util.ToPtr(""), Provider: common.RawJSON(`null`)}, "provider"},
		{"inner provider", ToolContentBlock{Type: ToolContentTypeResource, Resource: &ToolResourceContent{URI: "urn:report", Text: util.ToPtr(""), Provider: common.RawJSON(`[]`)}}, "provider"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.block.Validate(&common.ValidationContext{Path: "result.content[0]"})
			require.ErrorContains(t, err, test.field)
			require.ErrorContains(t, err, "result.content[0]")
		})
	}

	for _, priority := range []float64{-1, 1.1, math.NaN(), math.Inf(1)} {
		require.ErrorContains(t, (&ToolContentAnnotations{Priority: &priority}).Validate(nil), "priority")
	}
	require.ErrorContains(t, (&ToolContentAnnotations{Audience: []string{"system"}}).Validate(nil), "audience[0]")
	require.ErrorContains(t, (&ToolContentIcon{Src: "urn:icon", Theme: "blue"}).Validate(nil), "theme")
	require.Error(t, (*ToolContentBlock)(nil).Validate(nil))
}

// TestToolContentClone detaches every nested field used by admitted invocations
// so adapters and callers cannot mutate a captured result through shared data.
func TestToolContentClone(t *testing.T) {
	original := richToolContentForTest()
	clones := make([]ToolContentBlock, len(original))
	for i := range original {
		clones[i] = *original[i].Clone()
	}

	*clones[0].Text = "changed"
	clones[0].Provider[0] = '['
	clones[0].Annotations.Audience[0] = "changed"
	*clones[0].Annotations.Priority = 1
	*clones[1].Data = "changed"
	*clones[3].Name = "changed"
	*clones[3].Size = 1
	clones[3].Icons[0].Sizes[0] = "changed"
	*clones[4].Resource.Text = "changed"
	clones[4].Resource.Provider[0] = '['
	*clones[5].Resource.Blob = "changed"
	require.Equal(t, richToolContentForTest(), original)
	require.Nil(t, (*ToolContentBlock)(nil).Clone())
}

// TestToolContentYAMLBoundary keeps standalone block/resource decoding as
// strict as decoding a complete result, including explicit provider nulls.
func TestToolContentYAMLBoundary(t *testing.T) {
	block := richToolContentForTest()[0]
	before := block.Clone()
	require.Error(t, util.DecodeYAMLStrict([]byte("type: text\ntext: changed\nunknown: value"), &block))
	require.Equal(t, before, &block)
	require.NoError(t, util.DecodeYAMLStrict([]byte("type: text\ntext: ''\nprovider: null"), &block))
	require.ErrorContains(t, block.Validate(nil), "provider")

	var resource ToolResourceContent
	require.NoError(t, util.DecodeYAMLStrict([]byte("uri: urn:example\ntext: ''\nprovider: null"), &resource))
	require.ErrorContains(t, resource.Validate(nil), "provider")
	require.Error(t, util.DecodeYAMLStrict([]byte("uri: urn:example\ntext: ''\nunknown: value"), &resource))
}
