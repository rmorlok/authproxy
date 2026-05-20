package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// findURLArg has to pick the URL out of an arbitrary curl argv. Three
// forms matter: positional URL, --url <value>, --url=<value>. Anything
// that doesn't parse as an http(s) URL is ignored — flags whose values
// happen to look URL-ish (e.g. --data @body) must not be picked up.
func TestFindURLArg(t *testing.T) {
	t.Run("positional URL", func(t *testing.T) {
		idx, u, err := findURLArg([]string{"-X", "POST", "https://api.example.com/x", "-d", "{}"})
		require.NoError(t, err)
		assert.Equal(t, 2, idx)
		assert.Equal(t, "https://api.example.com/x", u.String())
	})

	t.Run("--url separated value", func(t *testing.T) {
		idx, u, err := findURLArg([]string{"--url", "https://api.example.com/y"})
		require.NoError(t, err)
		assert.Equal(t, 1, idx)
		assert.Equal(t, "https://api.example.com/y", u.String())
	})

	t.Run("--url=value attached", func(t *testing.T) {
		idx, u, err := findURLArg([]string{"-X", "GET", "--url=https://api.example.com/z"})
		require.NoError(t, err)
		assert.Equal(t, 2, idx)
		assert.Equal(t, "https://api.example.com/z", u.String())
	})

	t.Run("missing URL", func(t *testing.T) {
		_, _, err := findURLArg([]string{"-X", "POST", "-d", "@body"})
		require.Error(t, err)
	})

	t.Run("relative path is not a URL", func(t *testing.T) {
		_, _, err := findURLArg([]string{"/just/a/path"})
		require.Error(t, err)
	})

	t.Run("file URL is not picked up", func(t *testing.T) {
		_, _, err := findURLArg([]string{"file:///etc/hosts"})
		require.Error(t, err)
	})
}

// cmdRawProxyAlias must reuse cmdSigningProxy's flag surface so the old
// command keeps working byte-for-byte. The deprecation warning lives in
// RunE — verified by inspection rather than test since it goes to
// stderr.
func TestRawProxyAlias_KeepsSamePrimaryFlags(t *testing.T) {
	signing := cmdSigningProxy()
	alias := cmdRawProxyAlias()

	assert.Equal(t, "raw-proxy", alias.Use)
	assert.True(t, alias.Hidden, "alias must be hidden from help")
	for _, name := range []string{"proxyTo", "enableLoginRedirect", "port", "ip", "proto"} {
		assert.NotNil(t, signing.Flag(name), "signing-proxy must have flag %s", name)
		assert.NotNil(t, alias.Flag(name), "raw-proxy alias must have flag %s", name)
	}
}
