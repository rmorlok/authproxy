package oauth2

import (
	"strings"

	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util"
)

// SplitScopes parses a scope string of the shape RFC 6749 §3.3 defines —
// space-delimited tokens — into a slice of scope IDs. Returns an empty (but
// non-nil) slice when the input has no tokens, so JSON callers always see
// `[]` instead of `null`.
func SplitScopes(s string) []string {
	out := strings.Fields(s)
	if out == nil {
		return []string{}
	}
	return out
}

// JoinScopes is the inverse of SplitScopes: it joins the IDs of declared
// connector scopes into the wire format the authorize/token endpoints
// expect.
func JoinScopes(scopes []config.Scope) string {
	return strings.Join(util.Map(scopes, func(s config.Scope) string { return s.Id }), " ")
}

// scopeSet returns a presence-only map of the parsed scope tokens. Used by
// mismatch detection to check membership in O(1) without re-tokenizing.
func scopeSet(s string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, tok := range SplitScopes(s) {
		out[tok] = struct{}{}
	}
	return out
}
