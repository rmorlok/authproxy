package apjs

import (
	"time"

	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
)

// ConnectorPredicateLimits returns the finite execution profile for connection
// predicates used by probes, setup eligibility, and OAuth scope decisions, and
// for schema validation of their helper library and predicates. Each
// call returns an independent policy value; it does not change shared runtime
// defaults or grant capabilities. Five output bytes accommodate either boolean.
// HTTP requests and other connector expressions use their own separate policy.
func ConnectorPredicateLimits() jsruntime.Limits {
	return jsruntime.Limits{
		MaxSourceBytes: 256 << 10,
		MaxInputBytes:  1 << 20,
		MaxOutputBytes: 5,
		MaxDepth:       64,
		MaxNodes:       50000,
		MaxStackDepth:  256,
		Timeout:        time.Second,
	}
}
