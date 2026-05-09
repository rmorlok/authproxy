package ratelimit

import (
	"strconv"
	"strings"

	rlschema "github.com/rmorlok/authproxy/internal/schema/rate_limit"
)

// BucketKey is the projection of a matched request into independent
// counters. Components preserve the rule's dimension order so the
// String() form is deterministic across calls and across processes —
// callers can use it directly as a Redis sub-key.
type BucketKey struct {
	Components []BucketKeyComponent
}

// BucketKeyComponent pairs a dimension name with the value resolved from a
// request context. Empty Value means the dimension was unresolved (e.g.
// labels/<key> referenced a label not present on the request).
type BucketKeyComponent struct {
	Name  string
	Value string
}

// IsGlobal reports whether the rule had an empty dimensions list — meaning
// a single global counter for the entire rule rather than per-bucket ones.
func (k BucketKey) IsGlobal() bool {
	return len(k.Components) == 0
}

// String renders a stable, canonical string suitable for use as a Redis
// sub-key. The format escapes the field separator ('|') and the
// name-value separator ('=') in values to avoid collisions, e.g.
//
//	actor=act_abc|labels/team=alpha
//
// IsGlobal() keys render as "*".
func (k BucketKey) String() string {
	if k.IsGlobal() {
		return "*"
	}
	var b strings.Builder
	for i, c := range k.Components {
		if i > 0 {
			b.WriteByte('|')
		}
		b.WriteString(escapeBucketField(c.Name))
		b.WriteByte('=')
		b.WriteString(escapeBucketField(c.Value))
	}
	return b.String()
}

// escapeBucketField percent-encodes the two reserved separators so a
// label value containing '|' or '=' never produces an ambiguous key.
// Done by hand to keep the encoding minimal and stable; we don't want
// the broader URL-encoding rules quietly changing the output if a value
// happens to contain (e.g.) spaces or unicode.
func escapeBucketField(s string) string {
	if !strings.ContainsAny(s, "|=%") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 4)
	for _, r := range s {
		switch r {
		case '|':
			b.WriteString("%7C")
		case '=':
			b.WriteString("%3D")
		case '%':
			b.WriteString("%25")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ResolveBucketKey computes the bucket key for ctx according to the rule's
// bucket clause. An empty rule.Bucket.Dimensions list yields a global key
// (IsGlobal()). Reserved dimension names resolve to request-context fields;
// "labels/<key>" resolves to the request's per-call label snapshot. A
// dimension referencing data not present on the request resolves to "" —
// callers will see this as a distinct bucket from values that *are* present.
func ResolveBucketKey(rule rlschema.RateLimit, ctx *RequestContext) BucketKey {
	dims := rule.Bucket.Dimensions
	if len(dims) == 0 {
		return BucketKey{}
	}
	out := BucketKey{Components: make([]BucketKeyComponent, 0, len(dims))}
	for _, d := range dims {
		out.Components = append(out.Components, BucketKeyComponent{
			Name:  d,
			Value: resolveDimension(d, ctx),
		})
	}
	return out
}

// resolveDimension returns the per-request value for a single dimension.
// Empty string means "not present" — distinct from "explicitly empty" only
// in semantics; downstream counters treat the resolved tuple as opaque.
func resolveDimension(name string, ctx *RequestContext) string {
	if ctx == nil {
		return ""
	}
	switch name {
	case rlschema.DimensionActor:
		return string(ctx.ActorID)
	case rlschema.DimensionConnection:
		return string(ctx.ConnectionID)
	case rlschema.DimensionConnector:
		return string(ctx.ConnectorID)
	case rlschema.DimensionConnectorVersion:
		// Render as decimal so two connectors with versions 1 and 11
		// produce distinct buckets.
		if ctx.ConnectorVersion == 0 {
			return ""
		}
		return strconv.FormatUint(ctx.ConnectorVersion, 10)
	case rlschema.DimensionNamespace:
		return ctx.Namespace
	case rlschema.DimensionMethod:
		return ctx.Method
	}
	if strings.HasPrefix(name, rlschema.LabelDimensionPrefix) {
		key := strings.TrimPrefix(name, rlschema.LabelDimensionPrefix)
		if v, ok := ctx.Labels[key]; ok {
			return v
		}
		return ""
	}
	// Unknown dimension — schema validation should prevent this. Render as
	// empty so the resulting bucket key still serializes deterministically.
	return ""
}
