package apserde

import (
	"github.com/stretchr/testify/require"
	"testing"
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
	require.ElementsMatch(t, [][]string{{"credential"}, {"items", "0", "credential"}, {"lookup", "one", "credential"}}, SecretPaths(p))
}

func TestSecretPathsCyclesAndSharedValues(t *testing.T) {
	type recursive struct {
		Secret string     `json:"secret" apiredact:"secret"`
		Next   *recursive `json:"next"`
	}
	r := &recursive{}
	r.Next = r
	require.Equal(t, [][]string{{"secret"}}, SecretPaths(r))
	require.ElementsMatch(t, [][]string{{"0", "secret"}, {"1", "secret"}}, SecretPaths([]*recursive{r, r}))
}
