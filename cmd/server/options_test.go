package main

import (
	"testing"
	"time"

	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/require"
)

func TestServeOptionsServiceSelection(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  []sconfig.ServiceId
	}{
		{"all", sconfig.AllServiceIds()},
		{"admin-api,all,api,all", sconfig.AllServiceIds()},
		{" api, worker,api ", []sconfig.ServiceId{sconfig.ServiceIdApi, sconfig.ServiceIdWorker}},
	} {
		t.Run(tc.input, func(t *testing.T) {
			o, err := (serveFlags{}).resolve(tc.input)
			require.NoError(t, err)
			require.Equal(t, tc.want, o.services)
		})
	}
	for _, input := range []string{"", "api,", "all,unknown"} {
		_, err := (serveFlags{}).resolve(input)
		require.Error(t, err)
	}
}

func TestServeOptionsApplyIdentityAndService(t *testing.T) {
	flags := serveFlags{
		apply: startupApplyOptions{
			filenames: []string{"resources.yaml"},
			actorNamespace: "root",
			timeout: time.Minute,
		},
	}

	for _, tc := range []struct {
		services string
		want     sconfig.ServiceId
	}{
		{"all", sconfig.ServiceIdAdminApi},
		{"api,admin-api", sconfig.ServiceIdAdminApi},
		{"admin-api,api", sconfig.ServiceIdAdminApi},
		{"public,api", sconfig.ServiceIdApi},
	} {
		o, err := flags.resolve(tc.services)
		require.NoError(t, err)
		require.Equal(t, tc.want, o.applyService)
		require.Equal(t, "system", o.apply.actor)
		require.Equal(t, "root", o.apply.actorNamespace)
		require.True(t, o.apply.createSystemActor)
	}

	flags.apply.actor = "operator"
	flags.apply.actorNamespace = "root.ops"
	o, err := flags.resolve("all")
	require.NoError(t, err)
	require.False(t, o.apply.createSystemActor)
	require.Equal(t, "operator", o.apply.actor)
	require.Equal(t, "root.ops", o.apply.actorNamespace)

	flags.apply.filenames = nil
	o, err = flags.resolve("worker")
	require.NoError(t, err)
	require.False(t, o.apply.createSystemActor)
}
