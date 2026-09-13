package apserde

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSecretPathsIncludeEmptyNullAndContainers(t *testing.T) {
	type nested struct {
		Secret *string `json:"credential,omitempty" apiredact:"secret"`
	}
	type payload struct {
		nested
		Items  []nested          `json:"items"`
		Lookup map[string]nested `json:"lookup"`
	}
	p := payload{Items: []nested{{}}, Lookup: map[string]nested{"one": {}}}
	require.ElementsMatch(t, [][]string{
		{"credential"},
		{"items", "0", "credential"},
		{"lookup", "one", "credential"},
	}, SecretPaths(p))
}

func TestSecretPathsCyclesAndSharedValues(t *testing.T) {
	type recursive struct {
		Secret string     `json:"secret" apiredact:"secret"`
		Next   *recursive `json:"next"`
	}
	r := &recursive{}
	r.Next = r
	require.Equal(t, [][]string{{"secret"}}, SecretPaths(r))
	require.ElementsMatch(t, [][]string{
		{"0", "secret"},
		{"1", "secret"},
	}, SecretPaths([]*recursive{r, r}))
}

func TestSensitivePathsDiscoverUnregisteredResourceFields(t *testing.T) {
	type provider struct {
		Environment string `json:"environment"`
		Token       string `json:"token" apiredact:"secret"`
	}
	type spec struct {
		Credentials *provider `json:"credentials,omitempty" apiwriteonly:"true"`
		Visible     provider  `json:"visible"`
	}
	type resource struct {
		Spec spec `json:"spec"`
	}
	value := resource{Spec: spec{Credentials: &provider{Environment: "TOKEN_ENV", Token: "credential"}, Visible: provider{Environment: "PUBLIC_ENV", Token: "visible-secret"}}}
	require.Equal(t, [][]string{{"spec", "credentials"}}, WriteOnlyPaths(value))
	require.ElementsMatch(t, [][]string{{"spec", "credentials", "token"}, {"spec", "visible", "token"}}, SecretPaths(value))
	require.ElementsMatch(t, [][]string{{"spec", "credentials"}, {"spec", "credentials", "token"}, {"spec", "visible", "token"}}, SensitivePaths(value))
	// A nil write-only field is still known to consumers of a GET response.
	value.Spec.Credentials = nil
	require.Equal(t, [][]string{{"spec", "credentials"}}, WriteOnlyPaths(value))
}

func TestWriteOnlyPathsFollowContainersAndWrappers(t *testing.T) {
	type entry struct {
		Config *string `json:"config,omitempty" apiwriteonly:"true"`
	}
	type wrapper struct {
		InnerVal any `json:"-"`
	}
	value := struct {
		Embedded entry            `json:",inline"`
		Items    []entry          `json:"items"`
		Lookup   map[string]entry `json:"lookup"`
		Wrapped  wrapper          `json:"wrapped"`
	}{Items: []entry{{}}, Lookup: map[string]entry{"one": {}}, Wrapped: wrapper{InnerVal: entry{}}}
	require.ElementsMatch(t, [][]string{{"config"}, {"items", "0", "config"}, {"lookup", "one", "config"}, {"wrapped", "config"}}, WriteOnlyPaths(value))
}
