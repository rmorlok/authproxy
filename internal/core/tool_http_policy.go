package core

import (
	"errors"
	"math"
	"time"

	aphttp "github.com/rmorlok/authproxy/internal/apjs/http"
	"github.com/rmorlok/authproxy/internal/schema/common"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/tools/httpcodec"
	"github.com/rmorlok/authproxy/internal/tools/httpexec"
	"github.com/rmorlok/authproxy/internal/tools/javascript"
	"github.com/rmorlok/authproxy/internal/tools/render"
	"github.com/rmorlok/authproxy/internal/tools/validation"
)

// Tool transform budgets allow repeated native conversion passes and metadata
// visits while keeping utility work finite. These factors apply equally to work
// and allocation units; neither dimension measures actual VM heap usage.
const (
	toolTransformUnitsPerByte = 16
	toolTransformUnitsPerNode = 32
)

// toolHTTPInvocationPolicy resolves server configuration into a complete HTTP
// executor policy. Omitted fields use finite defaults; explicit invalid values
// fail even when an embedded caller skipped root configuration validation. The
// returned values are independent of configuration pointers and may later be
// lowered by the captured Tool's authored caps.
//
// Configuration remains a schema-layer contract. This translation belongs in
// core so config does not depend on execution packages or the connection host.
func (s *service) toolHTTPInvocationPolicy() (httpexec.Limits, error) {
	var settings *sconfig.ToolInvocationLimits
	if s.cfg != nil {
		if root := s.cfg.GetRoot(); root != nil && root.Tools != nil {
			settings = root.Tools.Invocation
		}
	}
	if err := settings.Validate(&common.ValidationContext{Path: "$.tools.invocation"}); err != nil {
		return httpexec.Limits{}, err
	}
	if settings == nil {
		settings = &sconfig.ToolInvocationLimits{}
	}

	timeout := 60 * time.Second
	if settings.Timeout != nil {
		timeout = settings.Timeout.Duration
	}
	responseBytes := toolPolicyBytes(settings.MaxResponseBytes, 8<<20)
	outputBytes := toolPolicyBytes(settings.MaxOutputBytes, 8<<20)
	sourceBytes := toolPolicyBytes(settings.MaxResponseSourceBytes, 256<<10)
	depth := toolPolicyInt(settings.MaxDepth, 64)
	values := toolPolicyInt(settings.MaxValues, 1_000_000)
	transform := javascript.Limits{
		MaxSourceBytes: sourceBytes,
		MaxInputBytes:  toolPolicyBytes(settings.MaxTransformInputBytes, 16<<20),
		MaxOutputBytes: outputBytes,
		MaxDepth:       depth,
		MaxNodes:       values,
		MaxStackDepth:  toolPolicyInt(settings.MaxStackDepth, 256),
		Timeout:        timeout,
	}
	units, err := toolTransformBudget(transform)
	if err != nil {
		return httpexec.Limits{}, err
	}
	transform.MaxWorkUnits, transform.MaxAllocationUnits = units, units

	// Arguments, copied configuration, response-transform input, and the result
	// envelope retain separate size limits. The transform also has one shared
	// work/allocation allowance across its conversions and pure utilities.
	return httpexec.Limits{
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
		Response: httpcodec.ResponseLimits{
			MaxSourceBytes:        sourceBytes,
			MaxResponseBytes:      responseBytes,
			MaxOutputBytes:        outputBytes,
			MaxEncodedResultBytes: toolPolicyBytes(settings.MaxEncodedResultBytes, 16<<20),
			Transform:             transform,
		},
		HTTP: aphttp.Limits{
			Timeout:          timeout,
			MaxRequests:      toolPolicyInt(settings.MaxRequests, 20),
			MaxResponseBytes: int64(responseBytes),
			MaxStreamBytes:   int64(toolPolicyBytes(settings.MaxResponseStreamBytes, 8<<20)),
			MaxHeaderBytes:   toolPolicyBytes(settings.MaxResponseHeaderBytes, 64<<10),
		},
	}, nil
}

// toolTransformBudget derives each cumulative allowance from the resolved
// input/output byte and node caps: 16*(input+output) + 32*nodes. The byte factor
// covers repeated native scans/copies; the node factor covers parsed values and
// inspected metadata. Remaining units are available to pure utilities, but
// arbitrary repeated helper calls can still exhaust the execution's allowance.
// Reject unrepresentable policies before multiplication/addition can overflow.
func toolTransformBudget(limits javascript.Limits) (int, error) {
	units := 0
	for _, term := range []struct {
		value  int // Resolved byte or node cap, before scaling.
		factor int // Logical work/allocation units per byte or node.
	}{
		{limits.MaxInputBytes, toolTransformUnitsPerByte},
		{limits.MaxOutputBytes, toolTransformUnitsPerByte},
		{limits.MaxNodes, toolTransformUnitsPerNode},
	} {
		if term.value <= 0 || term.value > (math.MaxInt-units)/term.factor {
			return 0, errors.New("tool response-transform input, output, and value limits cannot form a representable execution budget")
		}
		units += term.value * term.factor
	}
	return units, nil
}

// toolPolicyBytes converts a validated byte allowance to the executor's int
// representation. Only omission selects the default; validation rejects zero
// and sizes that cannot be represented before this helper is called.
func toolPolicyBytes(value *sconfig.HumanByteSize, fallback int) int {
	if value == nil {
		return fallback
	}
	return int(value.Value())
}

// toolPolicyInt returns a validated count or its default without retaining the
// configuration pointer. All supplied counts must be positive.
func toolPolicyInt(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}
