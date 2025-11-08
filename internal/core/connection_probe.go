package core

import (
	"errors"

	"github.com/rmorlok/authproxy/internal/core/iface"
)

var ErrProbeNotFound = errors.New("probe not found")

func (c *connection) GetProbe(probeId string) (iface.Probe, error) {
	def := c.cv.GetDefinition()

	for _, probe := range def.Probes {
		if probe.Id == probeId {
			return NewProbe(&probe, c.s, c.cv, c), nil
		}
	}

	return nil, ErrProbeNotFound
}

func (c *connection) GetProbes() []iface.Probe {
	def := c.cv.GetDefinition()
	probes := make([]iface.Probe, len(def.Probes))

	for _, probe := range def.Probes {
		probes = append(probes, NewProbe(&probe, c.s, c.cv, c))
	}

	return probes
}
