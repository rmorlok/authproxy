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
	def := c.cv.GetDefinition()

	for i := range def.Probes {
		if def.Probes[i].Id == probeId {
			return NewProbe(&def.Probes[i], c.s, c.cv, c), nil
		}
	}

	return nil, ErrProbeNotFound
}

func (c *connection) GetProbes() []iface.Probe {
	def := c.cv.GetDefinition()
	probes := make([]iface.Probe, 0, len(def.Probes))

	for i := range def.Probes {
		probes = append(probes, NewProbe(&def.Probes[i], c.s, c.cv, c))
	}

	return probes
}

func (c *connection) GetEnabledProbe(ctx context.Context, probeId string) (iface.Probe, error) {
	def := c.cv.GetDefinition()

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
		return NewProbe(probe, c.s, c.cv, c), nil
	}

	return nil, ErrProbeNotFound
}

func (c *connection) GetEnabledProbes(ctx context.Context) ([]iface.Probe, error) {
	def := c.cv.GetDefinition()
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
		probes = append(probes, NewProbe(probe, c.s, c.cv, c))
	}

	return probes, nil
}

func (c *connection) isProbeEnabled(ctx context.Context, probe *cschema.Probe) (bool, error) {
	if probe == nil || probe.If == nil {
		return true, nil
	}

	jsctx, err := c.GetJavascriptContext(ctx)
	if err != nil {
		return false, fmt.Errorf("probe %q: get javascript context: %w", probe.Id, err)
	}

	ok, err := probe.If.GetValue(jsctx)
	if err != nil {
		return false, fmt.Errorf("probe %q if.javascript: %w", probe.Id, err)
	}
	return ok, nil
}
