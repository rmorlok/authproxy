package api

import (
	"encoding/base64"
	"io"
	"math"
	"mime"
	"net/url"
	"slices"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util"
	"gopkg.in/yaml.v3"
)

// ToolContentType discriminates a rich block in the v1alpha1 result contract.
// Protocol adapters translate upstream types into this API-owned vocabulary.
type ToolContentType string

const (
	ToolContentTypeText         ToolContentType = "text"
	ToolContentTypeImage        ToolContentType = "image"
	ToolContentTypeAudio        ToolContentType = "audio"
	ToolContentTypeResourceLink ToolContentType = "resource_link"
	ToolContentTypeResource     ToolContentType = "resource"
)

// ToolContentBlock is the flattened, discriminated rich-content union. Only
// fields belonging to Type may be populated; annotations and provider metadata
// are shared by every variant. Text/data pointers distinguish empty from absent.
// Provider holds redacted upstream extension metadata as an opaque JSON object.
// It cannot supply instructions, permissions, or implicit resource fetching.
type ToolContentBlock struct {
	Type ToolContentType `json:"type" yaml:"type"`

	Text     *string `json:"text,omitempty" yaml:"text,omitempty"`
	Data     *string `json:"data,omitempty" yaml:"data,omitempty"`
	MIMEType string  `json:"mimeType,omitempty" yaml:"mimeType,omitempty"`

	URI         string            `json:"uri,omitempty" yaml:"uri,omitempty"`
	Name        *string           `json:"name,omitempty" yaml:"name,omitempty"`
	Title       string            `json:"title,omitempty" yaml:"title,omitempty"`
	Description string            `json:"description,omitempty" yaml:"description,omitempty"`
	Size        *int64            `json:"size,omitempty" yaml:"size,omitempty"`
	Icons       []ToolContentIcon `json:"icons,omitempty" yaml:"icons,omitempty"`

	Resource    *ToolResourceContent    `json:"resource,omitempty" yaml:"resource,omitempty"`
	Annotations *ToolContentAnnotations `json:"annotations,omitempty" yaml:"annotations,omitempty"`
	Provider    common.RawJSON          `json:"provider,omitempty" yaml:"provider,omitempty"`
}

// ToolResourceContent embeds exactly one text or base64 blob representation.
// URI is an identifier, including an invocation-scoped URI for generated inline
// data. It promises neither a download endpoint nor automatic dereferencing.
type ToolResourceContent struct {
	URI      string         `json:"uri" yaml:"uri"`
	MIMEType string         `json:"mimeType,omitempty" yaml:"mimeType,omitempty"`
	Text     *string        `json:"text,omitempty" yaml:"text,omitempty"`
	Blob     *string        `json:"blob,omitempty" yaml:"blob,omitempty"`
	Provider common.RawJSON `json:"provider,omitempty" yaml:"provider,omitempty"`
}

// ToolContentAnnotations preserves provider presentation hints. Audience and
// priority guide rendering only; neither conveys authority or changes Tool ACLs.
type ToolContentAnnotations struct {
	Audience     []string `json:"audience,omitempty" yaml:"audience,omitempty"`
	Priority     *float64 `json:"priority,omitempty" yaml:"priority,omitempty"`
	LastModified string   `json:"lastModified,omitempty" yaml:"lastModified,omitempty"`
}

// ToolContentIcon describes a resource-link icon without loading its source.
// Sizes retain provider strings (for example "48x48" or "any").
type ToolContentIcon struct {
	Src      string   `json:"src" yaml:"src"`
	MIMEType string   `json:"mimeType,omitempty" yaml:"mimeType,omitempty"`
	Sizes    []string `json:"sizes,omitempty" yaml:"sizes,omitempty"`
	Theme    string   `json:"theme,omitempty" yaml:"theme,omitempty"`
}

// Validate checks the union and finite payload encodings without decoding media,
// fetching URIs, sanitizing provider data, or enforcing runtime byte budgets.
func (b *ToolContentBlock) Validate(vc *common.ValidationContext) error {
	vc = toolResultValidationContext(vc)
	if b == nil {
		return vc.NewError("content block is required")
	}

	var result *multierror.Error
	linkFields := b.URI != "" || b.Name != nil || b.Title != "" || b.Description != "" || b.Size != nil || len(b.Icons) > 0
	invalidFields := false

	switch b.Type {
	case ToolContentTypeText:
		if b.Text == nil {
			result = multierror.Append(result, vc.NewErrorForField("text", "is required"))
		}
		invalidFields = b.Data != nil || b.MIMEType != "" || b.Resource != nil || linkFields
	case ToolContentTypeImage, ToolContentTypeAudio:
		if b.Data == nil {
			result = multierror.Append(result, vc.NewErrorForField("data", "is required"))
		} else {
			result = multierror.Append(result, validateToolBase64(*b.Data, vc.PushField("data")))
		}
		if b.MIMEType == "" {
			result = multierror.Append(result, vc.NewErrorForField("mimeType", "is required"))
		}
		invalidFields = b.Text != nil || b.Resource != nil || linkFields
	case ToolContentTypeResourceLink:
		result = multierror.Append(result, validateToolContentURI(b.URI, vc.PushField("uri")))
		if b.Name == nil {
			result = multierror.Append(result, vc.NewErrorForField("name", "is required"))
		}
		for i := range b.Icons {
			result = multierror.Append(result, b.Icons[i].Validate(vc.PushField("icons").PushIndex(i)))
		}
		invalidFields = b.Text != nil || b.Data != nil || b.Resource != nil
	case ToolContentTypeResource:
		result = multierror.Append(result, b.Resource.Validate(vc.PushField("resource")))
		invalidFields = b.Text != nil || b.Data != nil || b.MIMEType != "" || linkFields
	default:
		result = multierror.Append(result, vc.NewErrorForField("type", "must be text, image, audio, resource_link, or resource"))
	}

	if invalidFields {
		result = multierror.Append(result, vc.NewError("contains fields belonging to another content type"))
	}
	result = multierror.Append(result, validateToolMIMEType(b.MIMEType, vc.PushField("mimeType")))
	result = multierror.Append(result, b.Annotations.Validate(vc.PushField("annotations")))
	if err := validateOptionalToolJSON(b.Provider, true); err != nil {
		result = multierror.Append(result, vc.NewErrorForField("provider", err.Error()))
	}

	return result.ErrorOrNil()
}

// Validate requires one embedded representation and validates its URI, optional
// media type, base64 syntax, and opaque provider metadata without I/O.
func (r *ToolResourceContent) Validate(vc *common.ValidationContext) error {
	vc = toolResultValidationContext(vc)
	if r == nil {
		return vc.NewError("embedded resource is required")
	}

	var result *multierror.Error
	result = multierror.Append(result, validateToolContentURI(r.URI, vc.PushField("uri")))
	result = multierror.Append(result, validateToolMIMEType(r.MIMEType, vc.PushField("mimeType")))
	if (r.Text == nil) == (r.Blob == nil) {
		result = multierror.Append(result, vc.NewError("requires exactly one of text or blob"))
	}
	if r.Blob != nil {
		result = multierror.Append(result, validateToolBase64(*r.Blob, vc.PushField("blob")))
	}
	if err := validateOptionalToolJSON(r.Provider, true); err != nil {
		result = multierror.Append(result, vc.NewErrorForField("provider", err.Error()))
	}

	return result.ErrorOrNil()
}

// Validate accepts absent annotations and validates supported rendering hints.
// Timestamp spelling and audience order are preserved rather than normalized.
func (a *ToolContentAnnotations) Validate(vc *common.ValidationContext) error {
	if a == nil {
		return nil
	}
	vc = toolResultValidationContext(vc)
	var result *multierror.Error

	for i, audience := range a.Audience {
		if audience != "user" && audience != "assistant" {
			result = multierror.Append(result, vc.PushField("audience").PushIndex(i).NewError("must be user or assistant"))
		}
	}
	if a.Priority != nil && (math.IsNaN(*a.Priority) || math.IsInf(*a.Priority, 0) || *a.Priority < 0 || *a.Priority > 1) {
		result = multierror.Append(result, vc.NewErrorForField("priority", "must be between zero and one"))
	}

	return result.ErrorOrNil()
}

// Validate checks icon metadata without downloading an image or interpreting
// provider size hints as authorization to read files or network locations.
func (i *ToolContentIcon) Validate(vc *common.ValidationContext) error {
	vc = toolResultValidationContext(vc)
	if i == nil {
		return vc.NewError("icon is required")
	}
	var result *multierror.Error
	result = multierror.Append(result, validateToolContentURI(i.Src, vc.PushField("src")))
	result = multierror.Append(result, validateToolMIMEType(i.MIMEType, vc.PushField("mimeType")))
	if i.Theme != "" && i.Theme != "light" && i.Theme != "dark" {
		result = multierror.Append(result, vc.NewErrorForField("theme", "must be light or dark"))
	}
	return result.ErrorOrNil()
}

// validateToolContentURI accepts absolute identifiers, including custom schemes
// and fragments. It never resolves them or treats them as local file paths.
func validateToolContentURI(value string, vc *common.ValidationContext) error {
	parsed, err := url.Parse(value)
	if err != nil || !parsed.IsAbs() || strings.ContainsAny(value, " \t\r\n") {
		return vc.NewError("must be an absolute URI")
	}
	return nil
}

// validateToolMIMEType validates a supplied media type without restricting
// provider subtypes or discarding parameters. Empty means omitted.
func validateToolMIMEType(value string, vc *common.ValidationContext) error {
	if value == "" {
		return nil
	}
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil || !strings.Contains(mediaType, "/") {
		return vc.NewError("must be a media type")
	}
	return nil
}

// validateToolBase64 verifies standard padded base64 while streaming decoded
// bytes to discard, avoiding a second allocation proportional to payload size.
func validateToolBase64(value string, vc *common.ValidationContext) error {
	_, err := io.Copy(io.Discard, base64.NewDecoder(base64.StdEncoding.Strict(), strings.NewReader(value)))
	if err != nil {
		return vc.NewError("must be base64 data")
	}
	return nil
}

// Clone detaches every pointer, slice, and raw JSON value in a rich block.
func (b *ToolContentBlock) Clone() *ToolContentBlock {
	if b == nil {
		return nil
	}
	clone := *b
	clone.Text = util.CloneValue(b.Text)
	clone.Name = util.CloneValue(b.Name)
	clone.Data = util.CloneValue(b.Data)
	clone.Size = util.CloneValue(b.Size)
	clone.Provider = slices.Clone(b.Provider)
	clone.Icons = slices.Clone(b.Icons)
	for i := range clone.Icons {
		clone.Icons[i].Sizes = slices.Clone(b.Icons[i].Sizes)
	}
	clone.Annotations = util.CloneValue(b.Annotations)
	if clone.Annotations != nil {
		clone.Annotations.Audience = slices.Clone(b.Annotations.Audience)
		clone.Annotations.Priority = util.CloneValue(b.Annotations.Priority)
	}
	clone.Resource = util.CloneValue(b.Resource)
	if clone.Resource != nil {
		clone.Resource.Text = util.CloneValue(b.Resource.Text)
		clone.Resource.Blob = util.CloneValue(b.Resource.Blob)
		clone.Resource.Provider = slices.Clone(b.Resource.Provider)
	}
	return &clone
}

// UnmarshalYAML routes native YAML through lossless JSON conversion so null
// provider metadata is retained for validation instead of becoming omission.
func (b *ToolContentBlock) UnmarshalYAML(node *yaml.Node) error {
	var raw common.RawJSON
	if err := raw.UnmarshalYAML(node); err != nil {
		return err
	}
	var decoded ToolContentBlock
	if err := util.DecodeJSONStrict(raw, &decoded); err != nil {
		return err
	}
	*b = decoded
	return nil
}

// UnmarshalYAML preserves provider nulls and precision when an embedded resource
// is decoded directly, using the same boundary as complete results and blocks.
func (r *ToolResourceContent) UnmarshalYAML(node *yaml.Node) error {
	var raw common.RawJSON
	if err := raw.UnmarshalYAML(node); err != nil {
		return err
	}
	var decoded ToolResourceContent
	if err := util.DecodeJSONStrict(raw, &decoded); err != nil {
		return err
	}
	*r = decoded
	return nil
}
