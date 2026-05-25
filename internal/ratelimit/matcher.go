package ratelimit

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/common"
	rlschema "github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
)

// Match decides whether rule's selector matches ctx and, if so, computes
// the bucket key. All selector clauses are ANDed: a request must satisfy
// every non-empty clause for the rule to fire.
//
// Returns:
//   - matched=true with a populated BucketKey when the rule applies.
//   - matched=false with a zero BucketKey when the rule does not apply.
//   - a non-nil error only for malformed rule data that escaped schema
//     validation (e.g. an uncompilable regex). Callers should log + skip
//     such rules — they shouldn't be able to reach the runtime.
func Match(rule rlschema.RateLimit, ctx *RequestContext) (matched bool, key BucketKey, err error) {
	matched, key, _, err = MatchExplain(rule, ctx)
	return
}

// MatchExplain is like Match but, on a miss, returns a short reason string
// describing which selector clause refused. Powers the dry-run admin
// endpoint so operators can see why a rule didn't fire.
//
// Reason strings stay narrow and human-readable; callers display them
// verbatim. The same clause priority used by Match is preserved: the
// first clause that fails wins (request_type → method → label_selector
// → path_match).
func MatchExplain(rule rlschema.RateLimit, ctx *RequestContext) (matched bool, key BucketKey, reason string, err error) {
	if ctx == nil {
		return false, BucketKey{}, "request context not provided", nil
	}

	allowedTypes := rule.Selector.EffectiveRequestTypes()
	if !matchRequestType(allowedTypes, ctx.Type) {
		return false, BucketKey{}, fmt.Sprintf("request_type %q is not in the rule's allowed list", string(ctx.Type)), nil
	}

	if !matchMethods(rule.Selector.Methods, ctx.Method) {
		return false, BucketKey{}, fmt.Sprintf("method %q does not match the rule's method list", ctx.Method), nil
	}

	if rule.Selector.LabelSelector != "" {
		ok, lerr := matchLabelSelector(rule.Selector.LabelSelector, ctx.Labels)
		if lerr != nil {
			return false, BucketKey{}, "", fmt.Errorf("label selector: %w", lerr)
		}
		if !ok {
			return false, BucketKey{}, fmt.Sprintf("request labels do not satisfy %q", rule.Selector.LabelSelector), nil
		}
	}

	if rule.Selector.PathMatch != nil {
		var p string
		if ctx.UpstreamURL != nil {
			p = ctx.UpstreamURL.Path
		}
		ok, perr := matchPath(rule.Selector.PathMatch, p, ctx.UpstreamURL != nil)
		if perr != nil {
			return false, BucketKey{}, "", fmt.Errorf("path match: %w", perr)
		}
		if !ok {
			if !(ctx.UpstreamURL != nil) {
				return false, BucketKey{}, fmt.Sprintf("rule requires a %s path match but the request carried no URL", rule.Selector.PathMatch.Kind), nil
			}
			return false, BucketKey{}, fmt.Sprintf("path %q does not %s-match %q", p, rule.Selector.PathMatch.Kind, rule.Selector.PathMatch.Value), nil
		}
	}

	return true, ResolveBucketKey(rule, ctx), "", nil
}

// matchRequestType returns true if ctxType is in the rule's allowed list.
// allowed is the *effective* list — callers should pass
// rule.Selector.EffectiveRequestTypes() so the default [proxy, probe] is
// applied when the rule omits the field.
func matchRequestType(allowed []common.RequestType, ctxType common.RequestType) bool {
	if len(allowed) == 0 {
		// Defensive: EffectiveRequestTypes never returns an empty slice
		// because schema validation rejects an explicit empty list. If
		// we ever see one, fail closed (don't match).
		return false
	}
	for _, t := range allowed {
		if t == ctxType {
			return true
		}
	}
	return false
}

// matchMethods returns true when the rule has no method restriction
// (nil/empty list = any) or when the request's method is in the list.
// Comparison is exact (the schema rejects lowercase verbs at validation
// time, so a typo in either side fails to match here).
func matchMethods(allowed []string, ctxMethod string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, m := range allowed {
		if m == ctxMethod {
			return true
		}
	}
	return false
}

// matchLabelSelector parses + evaluates the rule's labelSelector against the
// per-request label snapshot. Parsing happens on every call rather than
// being cached — the enforcement layer can add caching later if profiling
// shows it matters.
func matchLabelSelector(selector string, labels map[string]string) (bool, error) {
	parsed, err := database.ParseLabelSelector(selector)
	if err != nil {
		return false, err
	}
	return parsed.Matches(labels), nil
}

// matchPath evaluates the rule's PathMatch against the path component of
// the resolved upstream URL. urlPresent=false means the request didn't
// carry a URL — a rule with a pathMatch clause then fails the match (the
// rule wants to filter on a path that doesn't exist).
//
// Glob semantics use Go's path.Match, where '*' does not cross '/' — same
// behavior as filepath.Match for shell globs. For full doublestar
// behaviour, callers can use kind=regex.
func matchPath(pm *rlschema.PathMatch, p string, urlPresent bool) (bool, error) {
	if pm == nil {
		return true, nil
	}
	if !urlPresent {
		return false, nil
	}
	switch pm.Kind {
	case rlschema.PathMatchKindPrefix:
		return strings.HasPrefix(p, pm.Value), nil
	case rlschema.PathMatchKindGlob:
		// path.Match returns ErrBadPattern for malformed globs. Schema
		// validation doesn't yet round-trip the glob through path.Match,
		// so propagate the error rather than silently failing closed.
		ok, err := path.Match(pm.Value, p)
		if err != nil {
			return false, err
		}
		return ok, nil
	case rlschema.PathMatchKindRegex:
		// Compile-on-call. The schema validator already proved it
		// compiles, so this is wasted work in steady state — fix when
		// the enforcement layer has a place to cache compiled regexes.
		re, err := regexp.Compile(pm.Value)
		if err != nil {
			return false, err
		}
		return re.MatchString(p), nil
	default:
		return false, fmt.Errorf("unknown path match kind %q", pm.Kind)
	}
}
