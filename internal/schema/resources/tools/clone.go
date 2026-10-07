package tools

import (
	"maps"
	"slices"

	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/util"
)

// Clone returns a detached Tool, including metadata, definition, and server
// status. It deliberately avoids serialization, preserving raw JSON bytes,
// explicit empty values, and validation state on malformed input.
func (t *Tool) Clone() *Tool {
	if t == nil {
		return nil
	}

	clone := *t
	clone.Metadata = meta.CloneObjectMeta(t.Metadata)
	clone.Spec = t.Spec.Clone()

	if t.Status != nil {
		clone.Status = util.CloneValue(t.Status)
		clone.Status.Conditions = slices.Clone(t.Status.Conditions)
		clone.Status.ManagedBy = util.CloneValue(t.Status.ManagedBy)
	}

	return &clone
}

// Clone returns a desired spec whose definition shares no mutable state with
// the original. ObjectReference itself contains only immutable scalar values.
func (s ToolSpec) Clone() ToolSpec {
	clone := s

	clone.ToolDefinition = *s.ToolDefinition.Clone()

	return clone
}

// Clone returns a detached authored definition for a patch or candidate
// revision. Maps, slices, raw JSON, and optional scalar pointers are copied so
// modifying a candidate never changes an already admitted definition snapshot.
func (d *ToolDefinition) Clone() *ToolDefinition {
	if d == nil {
		return nil
	}

	clone := *d
	clone.Verbs = slices.Clone(d.Verbs)
	clone.InputSchema = slices.Clone(d.InputSchema)
	clone.OutputSchema = slices.Clone(d.OutputSchema)
	clone.Javascript = util.CloneValue(d.Javascript)

	if d.Hints != nil {
		clone.Hints = util.CloneValue(d.Hints)
		clone.Hints.ReadOnly = util.CloneValue(d.Hints.ReadOnly)
		clone.Hints.Destructive = util.CloneValue(d.Hints.Destructive)
		clone.Hints.Idempotent = util.CloneValue(d.Hints.Idempotent)
	}

	if d.Limits != nil {
		clone.Limits = util.CloneValue(d.Limits)
		clone.Limits.TimeoutMillis = util.CloneValue(d.Limits.TimeoutMillis)
		clone.Limits.MaxRequests = util.CloneValue(d.Limits.MaxRequests)
		clone.Limits.MaxInputBytes = util.CloneValue(d.Limits.MaxInputBytes)
		clone.Limits.MaxOutputBytes = util.CloneValue(d.Limits.MaxOutputBytes)
		clone.Limits.MaxResponseBytes = util.CloneValue(d.Limits.MaxResponseBytes)
		clone.Limits.MaxEncodedResultBytes = util.CloneValue(d.Limits.MaxEncodedResultBytes)
		clone.Limits.MaxStackDepth = util.CloneValue(d.Limits.MaxStackDepth)
	}

	clone.ProxyHTTP = d.ProxyHTTP.Clone()

	return &clone
}

// Clone returns a detached HTTP plan, preserving each body mode and its
// wire-presence validation state. A nil plan remains nil; all mutable request
// and response mapping values are copied without interpreting raw JSON.
func (p *ProxyHTTP) Clone() *ProxyHTTP {
	if p == nil {
		return nil
	}

	clone := *p
	clone.Headers = maps.Clone(p.Headers)
	clone.Query = util.CloneRawJSONMap(p.Query)
	clone.BodyJSON = slices.Clone(p.BodyJSON)
	clone.BodyTemplate = util.CloneValue(p.BodyTemplate)
	clone.BodyRaw = util.CloneValue(p.BodyRaw)
	clone.nullBodyFields = slices.Clone(p.nullBodyFields)

	if p.Form != nil {
		clone.Form = &FormBody{Fields: util.CloneRawJSONMap(p.Form.Fields)}
	}

	if p.Multipart != nil {
		clone.Multipart = &MultipartBody{Parts: slices.Clone(p.Multipart.Parts)}
		for i := range clone.Multipart.Parts {
			clone.Multipart.Parts[i].Text = util.CloneValue(p.Multipart.Parts[i].Text)
			clone.Multipart.Parts[i].BodyRaw = util.CloneValue(p.Multipart.Parts[i].BodyRaw)
		}
	}

	if p.Response != nil {
		clone.Response = &HTTPResponse{
			TransformJavascript: util.CloneValue(p.Response.TransformJavascript),
			Errors:              slices.Clone(p.Response.Errors),
		}

		for i := range clone.Response.Errors {
			clone.Response.Errors[i].Statuses = slices.Clone(p.Response.Errors[i].Statuses)
			clone.Response.Errors[i].StatusClass = util.CloneValue(p.Response.Errors[i].StatusClass)
			clone.Response.Errors[i].Retryable = util.CloneValue(p.Response.Errors[i].Retryable)
		}
	}

	return &clone
}
