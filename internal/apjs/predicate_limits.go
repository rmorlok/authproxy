package apjs

import (
	"time"

	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
)

// ConnectorPredicateLimits returns the finite execution profile for connection
// predicates used by probes, setup eligibility, and OAuth scope decisions, and
// for schema validation of their helper library and predicates. Each
// call returns an independent policy value; it does not change shared runtime
// defaults or grant capabilities. Managed work and allocation each receive
// 64 Mi logical units across all conversions and pure utilities. Five output
// bytes accommodate either boolean.
// HTTP requests and other connector expressions use their own separate policy.
func ConnectorPredicateLimits() jsruntime.Limits {
	return jsruntime.Limits{
		MaxWorkUnits:       64 << 20,
		MaxAllocationUnits: 64 << 20,
		MaxSourceBytes:     256 << 10,
		MaxInputBytes:      1 << 20,
		MaxOutputBytes:     5,
		MaxDepth:           64,
		MaxNodes:           50000,
		MaxStackDepth:      256,
		Timeout:            time.Second,
	}
}

// ConnectorMigrationLimits returns the explicit policy for one generation's
// migration hook and its compile-only syntax validation. It shares the connector
// predicate source, input, traversal, stack, cumulative work/allocation, and
// execution bounds, with a separate
// 1 MiB JSON patch allowance.
// Multiple generations receive independent budgets under the common task context.
func ConnectorMigrationLimits() jsruntime.Limits {
	limits := ConnectorPredicateLimits()
	limits.MaxOutputBytes = 1 << 20
	return limits
}

// ConnectorDataSourceLimits returns the finite policy for one setup dropdown
// transform. Source, copied cfg/metadata/response inputs, traversal, stack, and
// timeout use the connector defaults. Both the intermediate JSON result and the
// final option array must fit 1 MiB independently, while both conversions share
// the cumulative work/allocation allowance. The HTTP fetch has its own
// existing policy; this timeout starts when JavaScript evaluation begins.
func ConnectorDataSourceLimits() jsruntime.Limits {
	limits := ConnectorPredicateLimits()
	limits.MaxOutputBytes = 1 << 20
	return limits
}
