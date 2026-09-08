package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const applyActor = "apiVersion: authproxy.net/v1alpha1\nkind: Actor\nmetadata:\n  name: bob\nspec: {}\n"

func TestApplyClientDryRun(t *testing.T) {
	for _, format := range []string{"", "name", "json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			cmd := cmdApply()
			var out bytes.Buffer
			cmd.SetIn(strings.NewReader(applyActor))
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			args := []string{"-f", "-", "--dry-run=client", "-n", "root.prod"}
			if format != "" {
				args = append(args, "-o", format)
			}
			cmd.SetArgs(args)
			require.NoError(t, cmd.Execute())
			require.Contains(t, out.String(), "bob")
			require.Contains(t, out.String(), "root.prod")
			if format == "" {
				require.Contains(t, out.String(), "validated (client dry run)")
			}
			require.NotContains(t, out.String(), "configured")
		})
	}
}
func TestApplyRejectsUnsupportedModesAndInvalidBatch(t *testing.T) {
	cases := [][]string{
		{"-f", "-"},
		{"-f", "-", "--dry-run=server"},
		{"-f", "-", "--dry-run=client", "--validate=ignore"},
		{"-f", "-", "--dry-run=client", "-o", "unknown"},
		{"--dry-run=client"},
		{"-f", "-", "--dry-run=client"}, // Missing namespace.
		{"-f", "-", "--dry-run=client", "--server-side"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			cmd := cmdApply()
			var out, stderr bytes.Buffer
			cmd.SetIn(strings.NewReader(applyActor))
			cmd.SetOut(&out)
			cmd.SetErr(&stderr)
			cmd.SilenceUsage = true
			cmd.SetArgs(args)
			require.Error(t, cmd.Execute())
			require.Empty(t, out.String())
		})
	}
	cmd := cmdApply()
	var out, stderr bytes.Buffer
	cmd.SetIn(strings.NewReader(applyActor + "---\n" + strings.Replace(applyActor, "kind: Actor", "kind: NotAResource", 1)))
	cmd.SetOut(&out)
	cmd.SetErr(&stderr)
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"-f", "-", "--dry-run=client", "-n", "root", "-o", "json"})
	require.ErrorContains(t, cmd.Execute(), "document 2")
	require.Empty(t, out.String())
}
func TestApplyOutputRedaction(t *testing.T) {
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			cmd := cmdApply()
			var out bytes.Buffer
			cmd.SetIn(strings.NewReader("apiVersion: authproxy.net/v1alpha1\nkind: Key\nmetadata: {name: example}\nspec: {keyData: {value: sensitive-key-material}}"))
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs([]string{"-f", "-", "--dry-run=client", "-n", "root", "-o", format})
			require.NoError(t, cmd.Execute())
			require.NotContains(t, out.String(), "sensitive-key-material")
			require.Contains(t, out.String(), "***")
		})
	}
}

type applyFailWriter struct{}

func (applyFailWriter) Write(p []byte) (int, error) { return 0, errors.New("output unavailable") }
func TestApplyOutputFailure(t *testing.T) {
	cmd := cmdApply()
	cmd.SetIn(strings.NewReader(applyActor))
	cmd.SetOut(applyFailWriter{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"-f", "-", "--dry-run=client", "-n", "root"})
	require.ErrorContains(t, cmd.Execute(), "output unavailable")
}
