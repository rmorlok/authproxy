package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApplyCustomPrinters(t *testing.T) {
	for _, tc := range []struct{ format, expression, want string }{
		{"go-template", `{{range .}}{{.kind}}{{"\n"}}{{end}}`, "Actor\n"},
		{"jsonpath", `{range [*]}{.kind}{"\n"}{end}`, "Actor\n"},
		{"jsonpath-as-json", `{[*].kind}`, "[\n    \"Actor\"\n]\n"},
	} {
		t.Run(tc.format, func(t *testing.T) {
			p, err := newApplyPrinter(tc.format, tc.expression, true)
			require.NoError(t, err)
			var out bytes.Buffer
			require.NoError(t, p(&out, []any{map[string]any{"kind": "Actor"}}))
			require.Equal(t, tc.want, out.String())
			file := filepath.Join(t.TempDir(), "template")
			require.NoError(t, os.WriteFile(file, []byte(tc.expression), 0600))
			if tc.format != "jsonpath-as-json" {
				p, err = newApplyPrinter(tc.format+"-file", file, true)
				require.NoError(t, err)
				out.Reset()
				require.NoError(t, p(&out, []any{map[string]any{"kind": "Actor"}}))
				require.Equal(t, tc.want, out.String())
			}
		})
	}
	for _, format := range []string{"go-template={{.missing}}", "jsonpath={.missing}"} {
		p, err := newApplyPrinter(format, "", false)
		require.NoError(t, err)
		var out bytes.Buffer
		require.Error(t, p(&out, map[string]any{}))
		require.Empty(t, out.String())
	}
	for _, format := range []string{"go-template={{", "jsonpath={[}", "unknown"} {
		_, err := newApplyPrinter(format, "", true)
		require.Error(t, err)
	}
}

func TestApplyPrinterRedactsAndValidatesBeforeExecution(t *testing.T) {
	cmd := cmdApply()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(bytes.NewBufferString("apiVersion: authproxy.net/v1alpha1\nkind: Actor\nmetadata: {name: worker, namespace: root}\nspec: {signingKey: {sharedKey: {value: TOPSECRET}}}"))
	cmd.SetArgs([]string{"-f", "-", "--dry-run=client", "-o", "go-template={{printf \"%v\" .}}"})
	require.NoError(t, cmd.Execute())
	require.NotContains(t, out.String(), "TOPSECRET")
}
