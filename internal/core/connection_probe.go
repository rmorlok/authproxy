package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/rmorlok/authproxy/internal/core/iface"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
)

var ErrProbeNotFound = errors.New("probe not found")
var ErrProbeDisabled = errors.New("probe disabled")

func (c *connection) GetProbe(probeId string) (iface.Probe, error) {
	def := c.connector.GetDefinition()

	for i := range def.Probes {
		if def.Probes[i].Id == probeId {
			return NewProbe(&def.Probes[i], c.s, c.connector, c), nil
		}
	}

	return nil, ErrProbeNotFound
}

func (c *connection) GetProbes() []iface.Probe {
	def := c.connector.GetDefinition()
	probes := make([]iface.Probe, 0, len(def.Probes))

	for i := range def.Probes {
		probes = append(probes, NewProbe(&def.Probes[i], c.s, c.connector, c))
	}

	return probes
}

func (c *connection) GetEnabledProbe(ctx context.Context, probeId string) (iface.Probe, error) {
	def := c.connector.GetDefinition()

	for i := range def.Probes {
		probe := &def.Probes[i]
		if probe.Id != probeId {
			continue
		}

		enabled, err := c.isProbeEnabled(ctx, probe)
		if err != nil {
			return nil, err
		}
		if !enabled {
			return nil, fmt.Errorf("probe %q: %w", probeId, ErrProbeDisabled)
		}
		return NewProbe(probe, c.s, c.connector, c), nil
	}

	return nil, ErrProbeNotFound
}

func (c *connection) GetEnabledProbes(ctx context.Context) ([]iface.Probe, error) {
	def := c.connector.GetDefinition()
	probes := make([]iface.Probe, 0, len(def.Probes))

	for i := range def.Probes {
		probe := &def.Probes[i]
		enabled, err := c.isProbeEnabled(ctx, probe)
		if err != nil {
			return nil, err
		}
		if !enabled {
			continue
		}
		probes = append(probes, NewProbe(probe, c.s, c.connector, c))
	}

	return probes, nil
}

// probePredicateLimits supplies finite caller policy for each probe condition.
// A predicate returns only a boolean, so five output bytes accommodate either
// value. These limits apply to the library, input, and condition together; probe
// HTTP requests retain their existing separate timeout and execution policy.
func probePredicateLimits() jsruntime.Limits {
	return jsruntime.Limits{
		MaxSourceBytes: 256 << 10,
		MaxInputBytes:  1 << 20,
		MaxOutputBytes: 5,
		MaxDepth:       64,
		MaxNodes:       50000,
		MaxStackDepth:  256,
		Timeout:        time.Second,
	}
}

// isProbeEnabled evaluates an optional condition against copied connection data
// under the request/task context and the probe's finite JavaScript policy. A
// condition failure disables execution by returning an error rather than guessing
// a boolean, and authored diagnostics remain behind safe runtime classifications.
func (c *connection) isProbeEnabled(ctx context.Context, probe *cschema.Probe) (bool, error) {
	if probe == nil || probe.If == nil {
		return true, nil
	}

	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("probe %q if.javascript: %w", probe.Id, err)
	}

	limits := probePredicateLimits()
	librarySource := c.connector.GetDefinition().Javascript

	if len(probe.If.Javascript) > limits.MaxSourceBytes ||
		len(librarySource) > limits.MaxSourceBytes-len(probe.If.Javascript) {
		return false, fmt.Errorf("probe %q if.javascript: %w", probe.Id, jsruntime.ErrLimitExceeded)
	}

	// Check combined source size before the connector cache can compile its
	// library. The public legacy context builder intentionally remains unchanged.
	library, err := c.connector.getJavascriptLibrary()

	// Compilation itself cannot be interrupted; preserve a cancellation that
	// arrived while it ran before reporting source errors or preparing input.
	if contextErr := ctx.Err(); contextErr != nil {
		return false, fmt.Errorf("probe %q if.javascript: %w", probe.Id, contextErr)
	}

	if err != nil {
		return false, fmt.Errorf("probe %q if.javascript: %w", probe.Id, jsruntime.ErrInvalidSource)
	}

	jsctx, err := c.getJavascriptContext(ctx, library)
	if err != nil {
		return false, fmt.Errorf("probe %q: get javascript context: %w", probe.Id, err)
	}

	ok, err := probe.If.GetValueWithContext(ctx, jsctx, limits)
	if err != nil {
		return false, fmt.Errorf("probe %q if.javascript: %w", probe.Id, err)
	}

	return ok, nil
}
