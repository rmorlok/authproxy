package tools

import (
	"maps"
	"slices"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
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
		clone.Status = cloneValue(t.Status)
		clone.Status.Conditions = slices.Clone(t.Status.Conditions)
		clone.Status.ManagedBy = cloneValue(t.Status.ManagedBy)
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
	clone.Javascript = cloneValue(d.Javascript)
	if d.Hints != nil {
		clone.Hints = cloneValue(d.Hints)
		clone.Hints.ReadOnly = cloneValue(d.Hints.ReadOnly)
		clone.Hints.Destructive = cloneValue(d.Hints.Destructive)
		clone.Hints.Idempotent = cloneValue(d.Hints.Idempotent)
	}
	if d.Limits != nil {
		clone.Limits = cloneValue(d.Limits)
		clone.Limits.TimeoutMillis = cloneValue(d.Limits.TimeoutMillis)
		clone.Limits.MaxRequests = cloneValue(d.Limits.MaxRequests)
		clone.Limits.MaxInputBytes = cloneValue(d.Limits.MaxInputBytes)
		clone.Limits.MaxOutputBytes = cloneValue(d.Limits.MaxOutputBytes)
		clone.Limits.MaxResponseBytes = cloneValue(d.Limits.MaxResponseBytes)
		clone.Limits.MaxEncodedResultBytes = cloneValue(d.Limits.MaxEncodedResultBytes)
		clone.Limits.MaxStackDepth = cloneValue(d.Limits.MaxStackDepth)
	}
	clone.ProxyHTTP = cloneProxyHTTP(d.ProxyHTTP)
	return &clone
}

// cloneProxyHTTP preserves each body mode and its wire-presence validation
// state while detaching all mutable request and response mapping values.
func cloneProxyHTTP(p *ProxyHTTP) *ProxyHTTP {
	if p == nil {
		return nil
	}
	clone := *p
	clone.Headers = maps.Clone(p.Headers)
	clone.Query = cloneRawJSONMap(p.Query)
	clone.BodyJSON = slices.Clone(p.BodyJSON)
	clone.BodyTemplate = cloneValue(p.BodyTemplate)
	clone.BodyRaw = cloneValue(p.BodyRaw)
	clone.nullBodyFields = slices.Clone(p.nullBodyFields)
	if p.Form != nil {
		clone.Form = &FormBody{Fields: cloneRawJSONMap(p.Form.Fields)}
	}
	if p.Multipart != nil {
		clone.Multipart = &MultipartBody{Parts: slices.Clone(p.Multipart.Parts)}
		for i := range clone.Multipart.Parts {
			clone.Multipart.Parts[i].Text = cloneValue(p.Multipart.Parts[i].Text)
			clone.Multipart.Parts[i].BodyRaw = cloneValue(p.Multipart.Parts[i].BodyRaw)
		}
	}
	if p.Response != nil {
		clone.Response = &HTTPResponse{
			TransformJavascript: cloneValue(p.Response.TransformJavascript),
			Errors:              slices.Clone(p.Response.Errors),
		}
		for i := range clone.Response.Errors {
			clone.Response.Errors[i].Statuses = slices.Clone(p.Response.Errors[i].Statuses)
			clone.Response.Errors[i].StatusClass = cloneValue(p.Response.Errors[i].StatusClass)
			clone.Response.Errors[i].Retryable = cloneValue(p.Response.Errors[i].Retryable)
		}
	}
	return &clone
}

// cloneRawJSONMap copies both the map and each raw JSON byte slice, retaining
// nil versus explicit empty values for validation and patch semantics.
func cloneRawJSONMap(values map[string]common.RawJSON) map[string]common.RawJSON {
	if values == nil {
		return nil
	}
	clone := make(map[string]common.RawJSON, len(values))
	for key, value := range values {
		clone[key] = slices.Clone(value)
	}
	return clone
}

// cloneValue copies a pointer to a scalar or shallow struct. Callers must
// separately detach any nested pointers, maps, or slices in structured values.
func cloneValue[T any](value *T) *T {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
