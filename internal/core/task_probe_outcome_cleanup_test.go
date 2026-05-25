package core

import (
	"testing"

	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/assert"
)

func TestProbeKeepMinimum_UsesLargerThreshold(t *testing.T) {
	p := &cschema.Probe{
		FailureThreshold:  intPtr(5),
		RecoveryThreshold: intPtr(2),
	}
	assert.Equal(t, 5, probeKeepMinimum(p))

	p = &cschema.Probe{
		FailureThreshold:  intPtr(1),
		RecoveryThreshold: intPtr(10),
	}
	assert.Equal(t, 10, probeKeepMinimum(p))
}

func TestProbeKeepMinimum_UsesDefaultsWhenUnset(t *testing.T) {
	p := &cschema.Probe{}
	want := cschema.DefaultProbeFailureThreshold
	if cschema.DefaultProbeRecoveryThreshold > want {
		want = cschema.DefaultProbeRecoveryThreshold
	}
	assert.Equal(t, want, probeKeepMinimum(p))
}
