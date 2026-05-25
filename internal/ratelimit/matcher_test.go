package ratelimit

import (
	"net/url"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/common"
	rlschema "github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
	"github.com/stretchr/testify/require"
)

// validRule returns a permissive rule that matches anything by default —
// individual tests narrow it via the closure passed to mutate.
func validRule(mutate func(*rlschema.RateLimit)) rlschema.RateLimit {
	r := rlschema.RateLimit{
		Selector: rlschema.Selector{}, // no clauses = match anything (subject to default request types)
		Bucket:   rlschema.Bucket{},
		Algorithm: rlschema.Algorithm{
			TokenBucket: &rlschema.TokenBucket{Capacity: 1, RefillRate: 1},
		},
	}
	if mutate != nil {
		mutate(&r)
	}
	return r
}

func proxyCtx(mutate func(*RequestContext)) *RequestContext {
	c := &RequestContext{Type: common.RequestTypeProxy, Method: "GET"}
	if mutate != nil {
		mutate(c)
	}
	return c
}

// --- Request-type matching ---

func TestMatch_RequestType_DefaultAcceptsProxy(t *testing.T) {
	matched, _, err := Match(validRule(nil), proxyCtx(func(c *RequestContext) { c.Type = common.RequestTypeProxy }))
	require.NoError(t, err)
	require.True(t, matched)
}

func TestMatch_RequestType_DefaultAcceptsProbe(t *testing.T) {
	matched, _, err := Match(validRule(nil), proxyCtx(func(c *RequestContext) { c.Type = common.RequestTypeProbe }))
	require.NoError(t, err)
	require.True(t, matched)
}

func TestMatch_RequestType_DefaultRejectsOAuth(t *testing.T) {
	// Default is [proxy, probe] — anything else is rejected unless the
	// rule explicitly opts in via Selector.RequestTypes.
	matched, _, err := Match(validRule(nil), proxyCtx(func(c *RequestContext) { c.Type = common.RequestTypeOAuth }))
	require.NoError(t, err)
	require.False(t, matched)
}

func TestMatch_RequestType_ExplicitOptIn(t *testing.T) {
	rule := validRule(func(r *rlschema.RateLimit) {
		r.Selector.RequestTypes = []common.RequestType{common.RequestTypeOAuth}
	})
	// Default proxy traffic now fails to match.
	matched, _, err := Match(rule, proxyCtx(func(c *RequestContext) { c.Type = common.RequestTypeProxy }))
	require.NoError(t, err)
	require.False(t, matched)

	// Opted-in oauth traffic matches.
	matched, _, err = Match(rule, proxyCtx(func(c *RequestContext) { c.Type = common.RequestTypeOAuth }))
	require.NoError(t, err)
	require.True(t, matched)
}

// --- Method matching ---

func TestMatch_Methods_EmptyMatchesAny(t *testing.T) {
	rule := validRule(func(r *rlschema.RateLimit) { r.Selector.Methods = nil })
	for _, m := range []string{"GET", "POST", "DELETE"} {
		matched, _, err := Match(rule, proxyCtx(func(c *RequestContext) { c.Method = m }))
		require.NoError(t, err)
		require.True(t, matched, "method %s should match an unrestricted rule", m)
	}
}

func TestMatch_Methods_FilterApplies(t *testing.T) {
	rule := validRule(func(r *rlschema.RateLimit) {
		r.Selector.Methods = []string{"POST", "PATCH"}
	})

	matched, _, err := Match(rule, proxyCtx(func(c *RequestContext) { c.Method = "POST" }))
	require.NoError(t, err)
	require.True(t, matched)

	matched, _, err = Match(rule, proxyCtx(func(c *RequestContext) { c.Method = "GET" }))
	require.NoError(t, err)
	require.False(t, matched)
}

// --- LabelSelector matching ---

func TestMatch_LabelSelector_Matches(t *testing.T) {
	rule := validRule(func(r *rlschema.RateLimit) {
		r.Selector.LabelSelector = "env=prod,team"
	})

	// Both clauses satisfied.
	matched, _, err := Match(rule, proxyCtx(func(c *RequestContext) {
		c.Labels = map[string]string{"env": "prod", "team": "platform"}
	}))
	require.NoError(t, err)
	require.True(t, matched)

	// Wrong env value.
	matched, _, err = Match(rule, proxyCtx(func(c *RequestContext) {
		c.Labels = map[string]string{"env": "staging", "team": "platform"}
	}))
	require.NoError(t, err)
	require.False(t, matched)

	// Missing team.
	matched, _, err = Match(rule, proxyCtx(func(c *RequestContext) {
		c.Labels = map[string]string{"env": "prod"}
	}))
	require.NoError(t, err)
	require.False(t, matched)
}

func TestMatch_LabelSelector_NotEqualAndNotExists(t *testing.T) {
	rule := validRule(func(r *rlschema.RateLimit) {
		r.Selector.LabelSelector = "env!=prod,!debug"
	})

	matched, _, err := Match(rule, proxyCtx(func(c *RequestContext) {
		c.Labels = map[string]string{"env": "staging"}
	}))
	require.NoError(t, err)
	require.True(t, matched)

	// debug present → !debug fails.
	matched, _, err = Match(rule, proxyCtx(func(c *RequestContext) {
		c.Labels = map[string]string{"env": "staging", "debug": "true"}
	}))
	require.NoError(t, err)
	require.False(t, matched)

	// env=prod fails the != clause.
	matched, _, err = Match(rule, proxyCtx(func(c *RequestContext) {
		c.Labels = map[string]string{"env": "prod"}
	}))
	require.NoError(t, err)
	require.False(t, matched)
}

func TestMatch_LabelSelector_BadSyntaxReturnsError(t *testing.T) {
	rule := validRule(func(r *rlschema.RateLimit) {
		// Empty key after the bang isn't a valid label selector.
		r.Selector.LabelSelector = "!="
	})
	matched, _, err := Match(rule, proxyCtx(nil))
	require.Error(t, err)
	require.False(t, matched)
}

// --- PathMatch matching ---

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return u
}

func TestMatch_PathMatch_Prefix(t *testing.T) {
	rule := validRule(func(r *rlschema.RateLimit) {
		r.Selector.PathMatch = &rlschema.PathMatch{
			Kind: rlschema.PathMatchKindPrefix, Value: "/services/data/",
		}
	})

	matched, _, err := Match(rule, proxyCtx(func(c *RequestContext) {
		c.UpstreamURL = mustURL(t, "https://api.example.com/services/data/v50/sobjects/Account")
	}))
	require.NoError(t, err)
	require.True(t, matched)

	matched, _, err = Match(rule, proxyCtx(func(c *RequestContext) {
		c.UpstreamURL = mustURL(t, "https://api.example.com/v1/users")
	}))
	require.NoError(t, err)
	require.False(t, matched)
}

func TestMatch_PathMatch_Glob(t *testing.T) {
	rule := validRule(func(r *rlschema.RateLimit) {
		r.Selector.PathMatch = &rlschema.PathMatch{
			Kind: rlschema.PathMatchKindGlob, Value: "/v1/users/*",
		}
	})

	cases := []struct {
		path  string
		want  bool
		label string
	}{
		{"/v1/users/abc", true, "single-segment match"},
		{"/v1/users/", true, "trailing slash matches empty segment"},
		{"/v1/users/abc/orders", false, "* does not cross /"},
		{"/v1/admin/abc", false, "wrong prefix"},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			matched, _, err := Match(rule, proxyCtx(func(c *RequestContext) {
				c.UpstreamURL = mustURL(t, "https://api.example.com"+tc.path)
			}))
			require.NoError(t, err)
			require.Equal(t, tc.want, matched, "path=%s", tc.path)
		})
	}
}

func TestMatch_PathMatch_Regex(t *testing.T) {
	rule := validRule(func(r *rlschema.RateLimit) {
		r.Selector.PathMatch = &rlschema.PathMatch{
			Kind: rlschema.PathMatchKindRegex, Value: `^/v1/users/[0-9]+$`,
		}
	})

	matched, _, err := Match(rule, proxyCtx(func(c *RequestContext) {
		c.UpstreamURL = mustURL(t, "https://api.example.com/v1/users/42")
	}))
	require.NoError(t, err)
	require.True(t, matched)

	matched, _, err = Match(rule, proxyCtx(func(c *RequestContext) {
		c.UpstreamURL = mustURL(t, "https://api.example.com/v1/users/forty-two")
	}))
	require.NoError(t, err)
	require.False(t, matched)
}

func TestMatch_PathMatch_NilURLFails(t *testing.T) {
	rule := validRule(func(r *rlschema.RateLimit) {
		r.Selector.PathMatch = &rlschema.PathMatch{
			Kind: rlschema.PathMatchKindPrefix, Value: "/x",
		}
	})
	matched, _, err := Match(rule, proxyCtx(func(c *RequestContext) { c.UpstreamURL = nil }))
	require.NoError(t, err)
	require.False(t, matched)
}

func TestMatch_PathMatch_BadRegexReturnsError(t *testing.T) {
	rule := validRule(func(r *rlschema.RateLimit) {
		r.Selector.PathMatch = &rlschema.PathMatch{
			Kind: rlschema.PathMatchKindRegex, Value: "[unterminated",
		}
	})
	matched, _, err := Match(rule, proxyCtx(func(c *RequestContext) {
		c.UpstreamURL = mustURL(t, "https://api.example.com/x")
	}))
	require.Error(t, err)
	require.False(t, matched)
}

func TestMatch_PathMatch_BadGlobReturnsError(t *testing.T) {
	rule := validRule(func(r *rlschema.RateLimit) {
		r.Selector.PathMatch = &rlschema.PathMatch{
			Kind: rlschema.PathMatchKindGlob, Value: "[unterminated",
		}
	})
	matched, _, err := Match(rule, proxyCtx(func(c *RequestContext) {
		c.UpstreamURL = mustURL(t, "https://api.example.com/x")
	}))
	require.Error(t, err)
	require.False(t, matched)
}

func TestMatch_PathMatch_UnknownKindReturnsError(t *testing.T) {
	rule := validRule(func(r *rlschema.RateLimit) {
		r.Selector.PathMatch = &rlschema.PathMatch{
			Kind: "exact", Value: "/x",
		}
	})
	matched, _, err := Match(rule, proxyCtx(func(c *RequestContext) {
		c.UpstreamURL = mustURL(t, "https://api.example.com/x")
	}))
	require.Error(t, err)
	require.False(t, matched)
}

// --- AND combinations ---

func TestMatch_AllClausesANDed(t *testing.T) {
	// All four clauses populated; one failure short-circuits the whole match.
	rule := validRule(func(r *rlschema.RateLimit) {
		r.Selector = rlschema.Selector{
			LabelSelector: "team=platform",
			Methods:       []string{"POST"},
			PathMatch: &rlschema.PathMatch{
				Kind: rlschema.PathMatchKindPrefix, Value: "/v1/",
			},
			RequestTypes: []common.RequestType{common.RequestTypeProxy},
		}
	})

	allGood := proxyCtx(func(c *RequestContext) {
		c.Method = "POST"
		c.Labels = map[string]string{"team": "platform"}
		c.UpstreamURL = mustURL(t, "https://api.example.com/v1/foo")
	})
	matched, _, err := Match(rule, allGood)
	require.NoError(t, err)
	require.True(t, matched)

	cases := []struct {
		name string
		mut  func(*RequestContext)
	}{
		{"wrong method", func(c *RequestContext) { c.Method = "GET" }},
		{"wrong label", func(c *RequestContext) { c.Labels = map[string]string{"team": "marketing"} }},
		{"wrong path", func(c *RequestContext) {
			c.UpstreamURL = mustURL(t, "https://api.example.com/v2/foo")
		}},
		{"wrong type", func(c *RequestContext) { c.Type = common.RequestTypeOAuth }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := proxyCtx(func(c *RequestContext) {
				c.Method = "POST"
				c.Labels = map[string]string{"team": "platform"}
				c.UpstreamURL = mustURL(t, "https://api.example.com/v1/foo")
				tc.mut(c)
			})
			matched, _, err := Match(rule, ctx)
			require.NoError(t, err)
			require.False(t, matched, "expected non-match when %s", tc.name)
		})
	}
}

// --- BucketKey on match ---

func TestMatch_ReturnsBucketKey(t *testing.T) {
	rule := validRule(func(r *rlschema.RateLimit) {
		r.Bucket.Dimensions = []string{
			rlschema.DimensionActor, "labels/team",
		}
	})
	ctx := proxyCtx(func(c *RequestContext) {
		c.ActorID = apid.ID("act_42")
		c.Labels = map[string]string{"team": "alpha"}
	})
	matched, k, err := Match(rule, ctx)
	require.NoError(t, err)
	require.True(t, matched)
	require.False(t, k.IsGlobal())
	require.Equal(t, "actor=act_42|labels/team=alpha", k.String())
}

func TestMatch_NonMatchReturnsZeroBucketKey(t *testing.T) {
	rule := validRule(func(r *rlschema.RateLimit) {
		r.Selector.Methods = []string{"POST"}
		r.Bucket.Dimensions = []string{rlschema.DimensionActor}
	})
	matched, k, err := Match(rule, proxyCtx(func(c *RequestContext) { c.Method = "GET" }))
	require.NoError(t, err)
	require.False(t, matched)
	require.True(t, k.IsGlobal(), "non-match should return a zero (global) BucketKey")
}

// --- Nil ctx is a no-match ---

func TestMatch_NilContext(t *testing.T) {
	matched, k, err := Match(validRule(nil), nil)
	require.NoError(t, err)
	require.False(t, matched)
	require.True(t, k.IsGlobal())
}
