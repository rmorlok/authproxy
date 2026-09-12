package apply

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnknownFieldModes(t *testing.T) {
	for _, mode := range []Validation{ValidationStrict, ValidationWarn, ValidationIgnore} {
		for _, tc := range []struct{ kind, spec string }{
			{"Actor", `{externalId: subject, unknownSecret: sensitive-value}`},
			{"Connector", `{definition: {displayName: Example, unknownSecret: sensitive-value, auth: {type: no-auth, unknownSecret: sensitive-value}}}`},
			{"Key", `{keyData: {value: real-secret, unknownSecret: sensitive-value}}`},
			{"RateLimit", `{algorithm: {tokenBucket: {capacity: 10, refillRate: 1, unknownSecret: sensitive-value}}}`},
		} {
			t.Run(string(mode)+tc.kind, func(t *testing.T) {
				var warnings []string
				input := manifestText(tc.kind, "  name: example\n  namespace: root\n  unknownSecret: sensitive-value", tc.spec) + "unknownSecret: sensitive-value\n"
				docs, err := Load(context.Background(), Options{Filenames: []string{"-"}, Stdin: strings.NewReader(input), Validation: mode, Warn: func(s string) { warnings = append(warnings, s) }})
				if mode == ValidationStrict {
					require.Error(t, err)
					return
				}
				require.NoError(t, err)
				require.Len(t, docs, 1)
				object, err := plainObject(docs[0].Object)
				require.NoError(t, err)
				require.NotContains(t, object, "unknownSecret")
				require.NotContains(t, object["metadata"], "unknownSecret")
				if mode == ValidationWarn {
					require.NotEmpty(t, warnings)
					require.NotContains(t, strings.Join(warnings, ""), "sensitive-value")
				} else {
					require.Empty(t, warnings)
				}
				// The filtered manifest must pass the default strict loader again.
				require.NotContains(t, docs[0].Object["spec"], "unknownSecret")
			})
		}
	}
}

func TestValidationModesRetainSafetyChecks(t *testing.T) {
	for _, mode := range []Validation{ValidationWarn, ValidationIgnore} {
		for _, input := range []string{
			manifestText("Actor", "  name: example", `{unknown: ignored}`),
			manifestText("Actor", "  name: example\n  namespace: null", `{}`),
			manifestText("Actor", "  name: example\n  namespace: root", `{externalId: [wrong-type]}`),
			manifestText("Actor", "  name: example\n  namespace: root", `{unknown: one, unknown: two}`),
			manifestText("Key", "  name: example\n  namespace: root", `{keyData: {value: '*****'}}`),
			manifestText("Key", "  name: example\n  namespace: root", `{keyData: {value: one, base64: dHdv}}`),
			manifestText("Actor", "  name: example\n  namespace: root", `{}`) + "status: {}\n",
			manifestText("Actor", "  name: example\n  namespace: root\n  createdAt: '2026-01-01T00:00:00Z'", `{}`),
			manifestText("Connector", "  name: example\n  namespace: root", `{definition: {auth: {type: nonsense}}}`),
		} {
			_, err := Load(context.Background(), Options{Filenames: []string{"-"}, Stdin: strings.NewReader(input), Validation: mode})
			require.Error(t, err, "%s: %s", mode, input)
		}
	}
}

func TestValidationListsAndArbitraryMaps(t *testing.T) {
	input := `apiVersion: authproxy.net/v1alpha1
kind: List
unknown: ignored
metadata: {unknown: ignored}
items:
- apiVersion: authproxy.net/v1alpha1
  kind: Actor
  metadata:
    name: example
    namespace: root
    labels: {custom: retained}
    annotations: {custom: retained}
  spec: {externalId: subject, unknown: ignored}
`
	for _, mode := range []Validation{ValidationWarn, ValidationIgnore} {
		docs, err := Load(context.Background(), Options{Filenames: []string{"-"}, Stdin: strings.NewReader(input), Validation: mode})
		require.NoError(t, err)
		require.Len(t, docs, 1)
		require.Equal(t, "retained", docs[0].Metadata.Labels["custom"])
		require.Equal(t, "retained", docs[0].Metadata.Annotations["custom"])
	}
}
