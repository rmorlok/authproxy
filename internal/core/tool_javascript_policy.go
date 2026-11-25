package core

import (
	"errors"
	"math"
	"time"

	aphttp "github.com/rmorlok/authproxy/internal/apjs/http"
	"github.com/rmorlok/authproxy/internal/tools/javascript"
	"github.com/rmorlok/authproxy/internal/tools/jsexec"
	"github.com/rmorlok/authproxy/internal/tools/render"
	"github.com/rmorlok/authproxy/internal/tools/validation"
)

// toolJavaScriptInvocationPolicy resolves operator ceilings for one JavaScript
// Tool. It shares common settings with HTTP Tools while retaining independent
// source, combined input, request, and worker allowances. Response-transform
// settings do not affect this profile. The returned policy owns only values;
// jsexec.Compile later lowers its ceilings using the captured Tool's limits.
// Core invocation dispatch must supply this policy before executing a Tool.
func (s *service) toolJavaScriptInvocationPolicy() (jsexec.Limits, error) {
	settings, err := s.toolInvocationSettings()
	if err != nil {
		return jsexec.Limits{}, err
	}

	timeout := 60 * time.Second
	if settings.Timeout != nil {
		timeout = settings.Timeout.Duration
	}
	outputBytes := toolPolicyBytes(settings.MaxOutputBytes, 8<<20)
	depth := toolPolicyInt(settings.MaxDepth, 64)
	values := toolPolicyInt(settings.MaxValues, 1_000_000)

	policy := jsexec.Limits{
		Render: render.Limits{
			MaxBytes: toolPolicyBytes(settings.MaxRenderBytes, 8<<20),
			MaxDepth: depth,
			MaxSteps: toolPolicyInt(settings.MaxRenderSteps, 1_000_000),
		},
		Validation: validation.Limits{
			MaxInputBytes:   toolPolicyBytes(settings.MaxInputBytes, 1<<20),
			MaxOutputBytes:  outputBytes,
			MaxDepth:        depth,
			MaxValues:       values,
			MaxNumberDigits: toolPolicyInt(settings.MaxNumberDigits, 4096),
		},
		Runtime: javascript.Limits{
			MaxSourceBytes: toolPolicyBytes(settings.MaxJavascriptSourceBytes, 256<<10),
			MaxInputBytes:  toolPolicyBytes(settings.MaxJavascriptInputBytes, 16<<20),
			MaxOutputBytes: outputBytes,
			MaxDepth:       depth,
			MaxNodes:       values,
			MaxStackDepth:  toolPolicyInt(settings.MaxStackDepth, 256),
			Timeout:        timeout,
		},
		HTTP: aphttp.Limits{
			Timeout:          timeout,
			MaxRequests:      toolPolicyInt(settings.MaxRequests, 20),
			MaxResponseBytes: int64(toolPolicyBytes(settings.MaxResponseBytes, 8<<20)),
			MaxStreamBytes:   int64(toolPolicyBytes(settings.MaxResponseStreamBytes, 8<<20)),
			MaxHeaderBytes:   toolPolicyBytes(settings.MaxResponseHeaderBytes, 64<<10),
		},
		MaxRequestBytes:       toolPolicyBytes(settings.MaxRequestBytes, 1<<20),
		MaxPendingOperations:  toolPolicyInt(settings.MaxPendingOperations, 8),
		MaxEncodedResultBytes: toolPolicyBytes(settings.MaxEncodedResultBytes, 16<<20),
	}
	units, err := toolJavaScriptBudget(policy)
	if err != nil {
		return jsexec.Limits{}, err
	}
	policy.Runtime.MaxWorkUnits, policy.Runtime.MaxAllocationUnits = units, units
	return policy, nil
}

// toolJavaScriptBudget derives each cumulative native allowance from admitted
// argument, VM input/output, and HTTP data: 16*(arguments+input+output+responses
// + requests*(requestBytes+headerBytes)) + 32*nodes. Request and header costs
// scale with logical requests; decoded response bytes are already aggregate.
// Argument bytes are separate because whitespace can disappear before VM input.
//
// These tuning factors allow repeated scans/copies and value metadata, as with
// response transforms. Utilities and repeated conversions still spend the same
// finite ledger; the formula is not a guarantee that arbitrary authored work
// fits, nor a heap quota. Check every product and sum before arithmetic so large
// positive settings cannot overflow into a smaller or unlimited budget.
func toolJavaScriptBudget(limits jsexec.Limits) (int, error) {
	if limits.HTTP.MaxResponseBytes <= 0 || limits.HTTP.MaxResponseBytes > int64(math.MaxInt) {
		return 0, errors.New("tool JavaScript response limit cannot form a representable execution budget")
	}
	units := 0
	for _, term := range []struct {
		value  int // Resolved byte or node allowance.
		count  int // Repetitions: logical requests for per-request data, otherwise one.
		factor int // Native work/allocation units per byte or node.
	}{
		{limits.Validation.MaxInputBytes, 1, 16},
		{limits.Runtime.MaxInputBytes, 1, 16},
		{limits.Runtime.MaxOutputBytes, 1, 16},
		{int(limits.HTTP.MaxResponseBytes), 1, 16},
		{limits.MaxRequestBytes, limits.HTTP.MaxRequests, 16},
		{limits.HTTP.MaxHeaderBytes, limits.HTTP.MaxRequests, 16},
		{limits.Runtime.MaxNodes, 1, 32},
	} {
		if term.value <= 0 || term.count <= 0 || term.value > (math.MaxInt-units)/term.factor/term.count {
			return 0, errors.New("tool JavaScript data and request limits cannot form a representable execution budget")
		}
		units += term.value * term.factor * term.count
	}
	return units, nil
}
