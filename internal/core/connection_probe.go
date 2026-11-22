package core

import (
	"context"
	"errors"
	"fmt"

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

// GetEnabledProbe loads the connector definition with the caller's context and
// returns the requested probe only when its condition passes. Definition-load
// errors remain wrapped causes; they neither panic nor become JavaScript errors.
func (c *connection) GetEnabledProbe(ctx context.Context, probeId string) (iface.Probe, error) {
	def, err := c.connector.getDefinitionWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("probe %q: load connector definition: %w", probeId, err)
	}

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

// GetEnabledProbes loads the connector definition and evaluates its conditions
// under the caller's context. Any load or predicate failure returns no probes,
// preserving the underlying error for classification and retry decisions.
func (c *connection) GetEnabledProbes(ctx context.Context) ([]iface.Probe, error) {
	def, err := c.connector.getDefinitionWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("load connector definition for probes: %w", err)
	}
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

// isProbeEnabled evaluates an optional condition with the shared connection
// predicate profile. An omitted condition enables the probe; evaluation failures
// remain errors so callers cannot mistake them for an ordinary false condition.
func (c *connection) isProbeEnabled(ctx context.Context, probe *cschema.Probe) (bool, error) {
	if probe == nil || probe.If == nil {
		return true, nil
	}
	ok, err := c.EvaluateJavascriptPredicate(ctx, probe.If.Javascript)
	if err != nil {
		return false, fmt.Errorf("probe %q if.javascript: %w", probe.Id, err)
	}
	return ok, nil
}
