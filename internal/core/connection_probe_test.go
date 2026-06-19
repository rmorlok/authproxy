package core

import (
	"context"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/assert"
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

func TestConnection_EnabledProbePredicates(t *testing.T) {
	conn := newTestConnection(cschema.Connector{
		Probes: []cschema.Probe{
			{
				Id: "cfg_true",
				If: &common.Predicate{Javascript: `cfg.region === "eu"`},
			},
			{
				Id: "cfg_false",
				If: &common.Predicate{Javascript: `cfg.region === "us"`},
			},
			{
				Id: "label_true",
				If: &common.Predicate{Javascript: `labels["apxy/cxr/type"] === "salesforce"`},
			},
			{
				Id: "annotation_true",
				If: &common.Predicate{Javascript: `annotations["setup-mode"] === "advanced"`},
			},
			{
				Id: "always",
			},
		},
	})
	setConnectionConfigFixture(t, conn, map[string]any{"region": "eu"})
	conn.Labels = map[string]string{"apxy/cxr/type": "salesforce"}
	conn.Annotations = map[string]string{"setup-mode": "advanced"}

	probes, err := conn.GetEnabledProbes(context.Background())
	require.NoError(t, err)
	require.Len(t, probes, 4)
	assert.Equal(t, "cfg_true", probes[0].GetId())
	assert.Equal(t, "label_true", probes[1].GetId())
	assert.Equal(t, "annotation_true", probes[2].GetId())
	assert.Equal(t, "always", probes[3].GetId())

	p, err := conn.GetEnabledProbe(context.Background(), "cfg_true")
	require.NoError(t, err)
	assert.Equal(t, "cfg_true", p.GetId())

	p, err = conn.GetEnabledProbe(context.Background(), "cfg_false")
	require.ErrorIs(t, err, ErrProbeDisabled)
	require.Nil(t, p)

	p, err = conn.GetEnabledProbe(context.Background(), "missing")
	require.ErrorIs(t, err, ErrProbeNotFound)
	require.Nil(t, p)
}

func TestConnection_EnabledProbePredicateError(t *testing.T) {
	conn := newTestConnection(cschema.Connector{
		Probes: []cschema.Probe{
			{
				Id: "broken",
				If: &common.Predicate{Javascript: `cfg.region ===`},
			},
		},
	})

	_, err := conn.GetEnabledProbes(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), `probe "broken" if.javascript`)
}
