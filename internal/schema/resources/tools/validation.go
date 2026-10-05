package tools

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
)

// Validate checks an authored definition without running JavaScript, rendering
// templates, or contacting a provider. Execution compilation must additionally
// validate source syntax, resolve template references, and enforce operator
// limits at admission time.
func (d *ToolDefinition) Validate(vc *common.ValidationContext) error {
	vc = validationContext(vc)
	if d == nil {
		return vc.NewError("tool definition is required")
	}
	var result *multierror.Error
	if strings.TrimSpace(d.Description) == "" {
		result = multierror.Append(result, vc.NewErrorForField("description", "is required"))
	}
	if len(d.Verbs) == 0 {
		result = multierror.Append(result, vc.NewErrorForField("verbs", "must contain at least one permission verb"))
	}
	seen := map[string]bool{}
	for i, verb := range d.Verbs {
		path := vc.PushField("verbs").PushIndex(i)
		if strings.TrimSpace(verb) == "" || strings.TrimSpace(verb) != verb {
			result = multierror.Append(result, path.NewError("must be a nonempty permission verb without surrounding whitespace"))
		}
		if seen[verb] {
			result = multierror.Append(result, path.NewError("must not duplicate another permission verb"))
		}
		seen[verb] = true
	}
	result = multierror.Append(result, validateSchema(d.InputSchema, true, vc.PushField("inputSchema")))
	if len(d.OutputSchema) > 0 {
		result = multierror.Append(result, validateSchema(d.OutputSchema, false, vc.PushField("outputSchema")))
	}
	if d.Limits != nil {
		result = multierror.Append(result, d.Limits.Validate(vc.PushField("limits")))
	}
	if (d.ProxyHTTP == nil) == (d.Javascript == nil) {
		result = multierror.Append(result, vc.NewError("must contain exactly one of proxyHttp or javascript"))
	}
	if d.ProxyHTTP != nil {
		result = multierror.Append(result, d.ProxyHTTP.Validate(vc.PushField("proxyHttp")))
	}
	if d.Javascript != nil && strings.TrimSpace(*d.Javascript) == "" {
		result = multierror.Append(result, vc.NewErrorForField("javascript", "must not be empty"))
	}
	return result.ErrorOrNil()
}

// Validate requires positive explicit caps. Applying defaults and comparing
// them with operator ceilings belongs to execution policy, not serialization.
func (l *ExecutionLimits) Validate(vc *common.ValidationContext) error {
	vc = validationContext(vc)
	if l == nil {
		return nil
	}
	var result *multierror.Error
	for _, field := range []struct {
		name  string
		value *int64
	}{
		{"timeoutMillis", l.TimeoutMillis}, {"maxInputBytes", l.MaxInputBytes},
		{"maxOutputBytes", l.MaxOutputBytes}, {"maxResponseBytes", l.MaxResponseBytes},
		{"maxEncodedResultBytes", l.MaxEncodedResultBytes},
	} {
		if field.value != nil && *field.value <= 0 {
			result = multierror.Append(result, vc.NewErrorForField(field.name, "must be positive"))
		}
	}
	if l.MaxRequests != nil && *l.MaxRequests <= 0 {
		result = multierror.Append(result, vc.NewErrorForField("maxRequests", "must be positive"))
	}
	if l.MaxStackDepth != nil && *l.MaxStackDepth <= 0 {
		result = multierror.Append(result, vc.NewErrorForField("maxStackDepth", "must be positive"))
	}
	return result.ErrorOrNil()
}

// Validate checks the static HTTP request shape, body exclusivity, and response
// rules. Final rendered URLs, headers, and encoded values are runtime checks.
func (p *ProxyHTTP) Validate(vc *common.ValidationContext) error {
	vc = validationContext(vc)
	if p == nil {
		return vc.NewError("HTTP executor is required")
	}
	var result *multierror.Error
	if !isHTTPToken(p.Method) {
		result = multierror.Append(result, vc.NewErrorForField("method", "must be an HTTP token"))
	}
	if strings.TrimSpace(p.URL) == "" {
		result = multierror.Append(result, vc.NewErrorForField("url", "is required"))
	}
	for name, value := range p.Headers {
		if !isHTTPToken(name) || strings.ContainsAny(value, "\r\n") {
			result = multierror.Append(result, vc.PushField("headers").PushField(name).NewError("must have a valid header name and no newline characters"))
		}
	}
	result = multierror.Append(result, validateFields(p.Query, vc.PushField("query")))
	for _, name := range p.nullBodyFields {
		result = multierror.Append(result, vc.NewErrorForField(name, "must not be null"))
	}
	bodies := 0
	if p.BodyJSON != nil {
		bodies++
		result = multierror.Append(result, validateJSONBody(p.BodyJSON, vc.PushField("bodyJson")))
	}
	if p.BodyTemplate != nil {
		bodies++
		result = multierror.Append(result, validateMediaType(p.BodyTemplate.MediaType, vc.PushField("bodyTemplate").PushField("mediaType")))
	}
	if p.BodyRaw != nil {
		bodies++
		result = multierror.Append(result, validateBase64(*p.BodyRaw, vc.PushField("bodyRaw")))
	}
	if p.Form != nil {
		bodies++
		if p.Form.Fields == nil {
			result = multierror.Append(result, vc.NewErrorForField("form.fields", "is required and must not be null"))
		}
		result = multierror.Append(result, validateFields(p.Form.Fields, vc.PushField("form").PushField("fields")))
	}
	if p.Multipart != nil {
		bodies++
		if len(p.Multipart.Parts) == 0 {
			result = multierror.Append(result, vc.NewErrorForField("multipart.parts", "must contain at least one part"))
		}
		for i, part := range p.Multipart.Parts {
			result = multierror.Append(result, part.Validate(vc.PushField("multipart").PushField("parts").PushIndex(i)))
		}
	}
	if bodies > 1 {
		result = multierror.Append(result, vc.NewError("must contain at most one of bodyJson, bodyTemplate, bodyRaw, form, or multipart"))
	}
	if p.Response != nil {
		result = multierror.Append(result, p.Response.Validate(vc.PushField("response")))
	}
	return result.ErrorOrNil()
}

// Validate checks one ordinary multipart part without treating its filename
// as a local path or opening its contents.
func (p MultipartPart) Validate(vc *common.ValidationContext) error {
	vc = validationContext(vc)
	var result *multierror.Error
	if strings.TrimSpace(p.Name) == "" || strings.ContainsAny(p.Name, "\r\n") {
		result = multierror.Append(result, vc.NewErrorForField("name", "must be nonempty without newline characters"))
	}
	if strings.ContainsAny(p.Filename, "\r\n") {
		result = multierror.Append(result, vc.NewErrorForField("filename", "must not contain newline characters"))
	}
	if p.MediaType != "" {
		result = multierror.Append(result, validateMediaType(p.MediaType, vc.PushField("mediaType")))
	}
	if (p.Text == nil) == (p.BodyRaw == nil) {
		result = multierror.Append(result, vc.NewError("must contain exactly one of text or bodyRaw"))
	}
	if p.BodyRaw != nil {
		result = multierror.Append(result, validateBase64(*p.BodyRaw, vc.PushField("bodyRaw")))
	}
	return result.ErrorOrNil()
}

// Validate checks response-transform presence and rejects ambiguous error
// selectors. Different priorities may overlap deliberately. The execution
// compiler validates expression syntax and reserved platform error codes.
func (r *HTTPResponse) Validate(vc *common.ValidationContext) error {
	vc = validationContext(vc)
	var result *multierror.Error
	if r.TransformJavascript != nil {
		if strings.TrimSpace(*r.TransformJavascript) == "" {
			result = multierror.Append(result, vc.NewErrorForField("transformJavascript", "must not be empty"))
		}
	}
	statuses, classes := map[int]bool{}, map[int]bool{}
	defaultSeen := false
	for i, rule := range r.Errors {
		path := vc.PushField("errors").PushIndex(i)
		selectors := 0
		if rule.Statuses != nil {
			selectors++
			if len(rule.Statuses) == 0 {
				result = multierror.Append(result, path.NewErrorForField("statuses", "must not be empty"))
			}
			for _, status := range rule.Statuses {
				if status < 100 || status > 599 || statuses[status] {
					result = multierror.Append(result, path.NewErrorForField("statuses", "must contain unique HTTP statuses from 100 through 599 across all exact rules"))
				}
				statuses[status] = true
			}
		}
		if rule.StatusClass != nil {
			selectors++
			class := *rule.StatusClass
			if class < 1 || class > 5 || classes[class] {
				result = multierror.Append(result, path.NewErrorForField("statusClass", "must be a unique class from 1 through 5"))
			}
			classes[class] = true
		}
		if rule.Default {
			selectors++
			if defaultSeen {
				result = multierror.Append(result, path.NewErrorForField("default", "only one default rule is allowed"))
			}
			defaultSeen = true
		}
		if selectors != 1 {
			result = multierror.Append(result, path.NewError("must contain exactly one of statuses, statusClass, or default: true"))
		}
		if strings.TrimSpace(rule.Code) == "" || strings.TrimSpace(rule.Code) != rule.Code {
			result = multierror.Append(result, path.NewErrorForField("code", "must be nonempty without surrounding whitespace"))
		}
		if strings.TrimSpace(rule.Message) == "" {
			result = multierror.Append(result, path.NewErrorForField("message", "is required"))
		}
	}
	return result.ErrorOrNil()
}

// validateFields accepts scalars, repeated scalar values, and typed lookups.
// Structured parameter values are encoded later, never pre-encoded here.
func validateFields(fields map[string]common.RawJSON, vc *common.ValidationContext) error {
	var result *multierror.Error
	for key, raw := range fields {
		path := vc.PushField(key)
		value, err := decodeJSONValue(raw)
		if err != nil {
			result = multierror.Append(result, path.NewError("must contain valid JSON"))
			continue
		}
		if err := validateFieldValue(value, path); err != nil {
			result = multierror.Append(result, err)
		}
	}
	return result.ErrorOrNil()
}

// validateFieldValue ensures a query/form value has an unambiguous scalar or
// repeated-value encoding. A typed lookup is checked again after rendering.
func validateFieldValue(value any, vc *common.ValidationContext) error {
	switch typed := value.(type) {
	case nil, string, bool, json.Number:
		return nil
	case []any:
		for i, item := range typed {
			if _, nested := item.([]any); nested {
				return vc.PushIndex(i).NewError("must be a scalar or typed lookup")
			}
			if err := validateFieldValue(item, vc.PushIndex(i)); err != nil {
				return err
			}
		}
		return nil
	case map[string]any:
		if len(typed) == 1 {
			if reference, ok := typed["$value"].(string); ok && strings.TrimSpace(reference) != "" {
				return nil
			}
		}
	}
	return vc.NewError("must be a scalar, repeated scalars, or a single-key $value lookup")
}

// validateJSONBody checks typed directives while retaining arbitrary JSON
// values. A $literal directive protects its entire value from interpretation.
func validateJSONBody(raw common.RawJSON, vc *common.ValidationContext) error {
	value, err := decodeJSONValue(raw)
	if err != nil {
		return vc.NewError("must contain valid JSON")
	}
	if !schemaJSONDepthAllowed(value, 64) {
		return vc.NewError("JSON body nesting exceeds 64 levels")
	}
	return validateBodyValue(value, vc, 0)
}

// decodeJSONValue accepts exactly one JSON value without narrowing authored
// numbers to float64. The original bytes remain the serialized contract.
func decodeJSONValue(raw common.RawJSON) (any, error) {
	if !json.Valid(raw) {
		return nil, fmt.Errorf("must contain valid JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

// validateBodyValue bounds recursion and interprets only exact single-key
// directive objects; ordinary objects with similarly named keys stay literal.
func validateBodyValue(value any, vc *common.ValidationContext, depth int) error {
	if depth > 64 {
		return vc.NewError("JSON body nesting exceeds 64 levels")
	}
	switch typed := value.(type) {
	case map[string]any:
		if len(typed) == 1 {
			if _, literal := typed["$literal"]; literal {
				return nil
			}
			if reference, directive := typed["$value"]; directive {
				text, ok := reference.(string)
				if !ok || strings.TrimSpace(text) == "" {
					return vc.NewErrorForField("$value", "must be a nonempty context reference")
				}
				return nil
			}
		}
		for key, item := range typed {
			if err := validateBodyValue(item, vc.PushField(key), depth+1); err != nil {
				return err
			}
		}
	case []any:
		for i, item := range typed {
			if err := validateBodyValue(item, vc.PushIndex(i), depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateBase64 checks literal data immediately. Templates are decoded only
// after rendering; no validation path opens files or buffers an HTTP response.
func validateBase64(value string, vc *common.ValidationContext) error {
	if strings.Contains(value, "{{") {
		return nil
	}
	if _, err := base64.StdEncoding.Strict().DecodeString(value); err != nil {
		return vc.NewError("must be base64 data or a string template")
	}
	return nil
}

// validateMediaType requires a concrete media type so request encoding can be
// selected before rendering body text.
func validateMediaType(value string, vc *common.ValidationContext) error {
	if _, _, err := mime.ParseMediaType(value); err != nil {
		return vc.NewError("must be a valid media type")
	}
	return nil
}

// isHTTPToken accepts extension methods and header names supported by HTTP;
// restricting methods to an enumeration would narrow existing proxy behavior.
func isHTTPToken(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", char) {
			continue
		}
		return false
	}
	return true
}

// validationContext supplies a stable root for callers that omit a path.
func validationContext(vc *common.ValidationContext) *common.ValidationContext {
	if vc == nil {
		return &common.ValidationContext{Path: "$"}
	}
	return vc
}
