package core

import (
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/stretchr/testify/require"
)

func TestConnection_Probe(t *testing.T) {
	noProbes := newTestConnection(cschema.Connector{})
	hasProbes := newTestConnection(cschema.Connector{
		Probes: []cschema.Probe{
			{
				Id:     "probe-1",
				Period: common.HumanDurationFor("30m"),
			},
			{
				Id:     "probe-2",
				Period: common.HumanDurationFor("90m"),
			},
		},
	})

	t.Run("get probe", func(t *testing.T) {
		p, err := noProbes.GetProbe("does-not-exist")
		require.ErrorIs(t, err, ErrProbeNotFound)
		require.Nil(t, p)

		p, err = hasProbes.GetProbe("probe-2")
		require.NoError(t, err)
		require.Equal(t, "probe-2", p.GetId())

		p, err = hasProbes.GetProbe("does-not-exist")
		require.ErrorIs(t, err, ErrProbeNotFound)
		require.Nil(t, p)
	})
	t.Run("get probes", func(t *testing.T) {
		require.Empty(t, noProbes.GetProbes())

		probes := hasProbes.GetProbes()
		require.Len(t, probes, 2)
		require.Equal(t, "probe-1", probes[0].GetId())
		require.Equal(t, "probe-2", probes[1].GetId())
	})
}
