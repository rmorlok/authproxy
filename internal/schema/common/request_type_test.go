package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRequestType_IsValid(t *testing.T) {
	cases := []struct {
		name string
		t    RequestType
		want bool
	}{
		{"global", RequestTypeGlobal, true},
		{"proxy", RequestTypeProxy, true},
		{"oauth", RequestTypeOAuth, true},
		{"public", RequestTypePublic, true},
		{"probe", RequestTypeProbe, true},
		{"oauth2_token_exchange", RequestTypeOAuth2TokenExchange, true},
		{"oauth2_refresh", RequestTypeOAuth2Refresh, true},
		{"oauth2_revocation", RequestTypeOAuth2Revocation, true},
		{"empty", "", false},
		{"unknown", "bogus", false},
		{"case-mismatch", "Proxy", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, IsValidRequestType(tc.t))
			// String-typed callers should get the same result.
			require.Equal(t, tc.want, IsValidRequestType(string(tc.t)))
		})
	}
}

func TestRequestType_Validate(t *testing.T) {
	require.NoError(t, RequestType("proxy").Validate())
	require.NoError(t, RequestTypeOAuth2Refresh.Validate())

	err := RequestType("nope").Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), `"nope"`)
}

func TestAllRequestTypes(t *testing.T) {
	all := AllRequestTypes()
	require.Len(t, all, 8)
	for _, rt := range all {
		require.True(t, IsValidRequestType(rt), "expected %q to be valid", rt)
	}
}

func TestRequestType_String(t *testing.T) {
	require.Equal(t, "proxy", RequestTypeProxy.String())
}
