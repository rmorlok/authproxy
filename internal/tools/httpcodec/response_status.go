package httpcodec

import (
	"errors"
	"strconv"
	"unicode/utf8"

	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
)

// responseRule is an immutable, administrator-authored public error mapping.
// Its message is literal safe output, never a template or an upstream body.
type responseRule struct {
	// code identifies the provider-specific failure without impersonating a platform code.
	code string
	// message is the safe literal description exposed to the tool caller.
	message string
	// retryable is advice for the caller and never initiates another request.
	retryable bool
}

// responseRules captures validated selectors without retaining mutable source
// slices or pointers. Concurrent lookups only read the compiled tables.
type responseRules struct {
	// exact takes priority over a class and the fallback for the same status.
	exact map[int]responseRule
	// classes indexes HTTP status classes from 1 through 5.
	classes map[int]responseRule
	// fallback applies to every valid status, including successful 2xx responses.
	fallback *responseRule
}

// The valid selector space contains 500 exact statuses, five classes, and one
// default. These bounds reject oversized invalid input before schema validation
// allocates duplicate-detection maps or accumulates validation errors.
const (
	maxResponseRules         = 506
	maxResponseExactStatuses = 500
)

// compileResponseRules validates and captures the response error policy. A nil
// source supplies no custom rules. Transforms require a JavaScript runtime and
// are explicitly unsupported by this compiler, including an empty expression.
//
// maxSourceBytes must be positive. It caps combined code/message bytes, selector
// and retryable scalar text, and one selector marker per rule. It is a bounded
// source-work allowance, not the serialized JSON size or an exact memory cap.
func compileResponseRules(source *tools.HTTPResponse, maxSourceBytes int) (*responseRules, error) {
	if maxSourceBytes <= 0 {
		return nil, errors.New("response source byte limit must be positive")
	}
	if source == nil {
		return &responseRules{}, nil
	}
	if source.TransformJavascript != nil {
		return nil, errors.New("response JavaScript transforms are not supported")
	}
	if len(source.Errors) > maxResponseRules {
		return nil, errors.New("response error rule count exceeds the supported selector space")
	}

	// Check slice lengths before walking any authored statuses, and subtract
	// from the remaining budget to avoid overflowing an aggregate length.
	remainingStatuses := maxResponseExactStatuses
	for _, rule := range source.Errors {
		if len(rule.Statuses) > remainingStatuses {
			return nil, errors.New("response exact status count exceeds the supported selector space")
		}
		remainingStatuses -= len(rule.Statuses)
	}

	remainingBytes := maxSourceBytes
	charge := func(size int) bool {
		if size > remainingBytes {
			return false
		}
		remainingBytes -= size
		return true
	}
	for _, rule := range source.Errors {
		if !charge(1) || !charge(len(rule.Code)) || !charge(len(rule.Message)) {
			return nil, errors.New("response source exceeds the byte limit")
		}
		for _, status := range rule.Statuses {
			if !charge(len(strconv.Itoa(status))) {
				return nil, errors.New("response source exceeds the byte limit")
			}
		}
		if rule.StatusClass != nil && !charge(len(strconv.Itoa(*rule.StatusClass))) {
			return nil, errors.New("response source exceeds the byte limit")
		}
		if rule.Default && !charge(len("true")) {
			return nil, errors.New("response source exceeds the byte limit")
		}
		if rule.Retryable != nil && !charge(len(strconv.FormatBool(*rule.Retryable))) {
			return nil, errors.New("response source exceeds the byte limit")
		}
	}

	// Validate public strings only after the byte bound limits the scan. Schema
	// validation below owns selector shape, priority overlaps, and empty fields.
	for _, rule := range source.Errors {
		if !utf8.ValidString(rule.Code) || !utf8.ValidString(rule.Message) {
			return nil, errors.New("response error codes and messages must be valid UTF-8")
		}
		if isReservedResponseCode(rule.Code) {
			return nil, errors.New("response error rules cannot use reserved platform codes")
		}
	}
	if err := source.Validate(nil); err != nil {
		return nil, err
	}

	result := &responseRules{
		exact:   make(map[int]responseRule),
		classes: make(map[int]responseRule),
	}
	for _, sourceRule := range source.Errors {
		rule := responseRule{code: sourceRule.Code, message: sourceRule.Message}
		if sourceRule.Retryable != nil {
			rule.retryable = *sourceRule.Retryable
		}
		for _, status := range sourceRule.Statuses {
			result.exact[status] = rule
		}
		if sourceRule.StatusClass != nil {
			result.classes[*sourceRule.StatusClass] = rule
		}
		if sourceRule.Default {
			result.fallback = &rule
		}
	}
	return result, nil
}

// lookup returns the highest-priority mapping for a valid HTTP status. The
// returned value cannot mutate the compiled policy. Nil policies and statuses
// outside 100 through 599 have no match, even when a fallback exists.
func (r *responseRules) lookup(status int) (responseRule, bool) {
	if r == nil || status < 100 || status > 599 {
		return responseRule{}, false
	}
	if rule, found := r.exact[status]; found {
		return rule, true
	}
	if rule, found := r.classes[status/100]; found {
		return rule, true
	}
	if r.fallback != nil {
		return *r.fallback, true
	}
	return responseRule{}, false
}

// isReservedResponseCode protects platform failures from being impersonated by
// authored HTTP status mappings. Custom codes otherwise retain their spelling.
func isReservedResponseCode(code string) bool {
	switch code {
	case "INPUT_INVALID", "CONNECTION_UNAVAILABLE", "UPSTREAM_AUTH_FAILED",
		"UPSTREAM_FAILED", "UPSTREAM_PROTOCOL_ERROR", "UNSUPPORTED_CAPABILITY",
		"EXECUTION_LIMIT_EXCEEDED", "OUTPUT_INVALID", "OUTPUT_TOO_LARGE",
		"TOOL_REVISION_CHANGED":
		return true
	default:
		return false
	}
}
