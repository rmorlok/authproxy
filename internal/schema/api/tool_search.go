package api

import (
	"fmt"

	"github.com/hashicorp/go-multierror"
	toolsetschema "github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
	"github.com/rmorlok/authproxy/internal/util"
)

const (
	// DefaultToolSearchLimit keeps ordinary discovery responses small. Clients
	// loading all permitted tools must follow every continuation token.
	DefaultToolSearchLimit = 5

	// MaxToolSearchLimit bounds one discovery page, not the authorized inventory.
	MaxToolSearchLimit = 100
)

// ToolSearchQuery selects one page of public usage descriptors. Selection can
// only narrow the caller's permissions; the service filters authorization before
// pagination and binds continuation tokens to the principal and catalog context.
type ToolSearchQuery struct {
	Query string `json:"query,omitempty" yaml:"query,omitempty" form:"query"`

	// LabelSelector uses the shared label syntax, parsed by the discovery
	// service. Connection selection uses apxy/cxn/-/id=<connection-id> here.
	LabelSelector string           `json:"labelSelector,omitempty" yaml:"labelSelector,omitempty" form:"labelSelector"`
	ToolSetID     string           `json:"toolSetId,omitempty" yaml:"toolSetId,omitempty" form:"toolSetId"`
	Schema        ToolSchemaDetail `json:"schema,omitempty" yaml:"schema,omitempty" form:"schema"`

	// IncludeUnavailable includes permitted tools with unavailable or unknown
	// readiness. It does not change admission or authorize execution.
	IncludeUnavailable bool `json:"includeUnavailable,omitempty" yaml:"includeUnavailable,omitempty" form:"includeUnavailable"`
	Limit              *int `json:"limit,omitempty" yaml:"limit,omitempty" form:"limit"`

	// Continue is opaque. Validation does not decode or normalize its contents.
	Continue string `json:"continue,omitempty" yaml:"continue,omitempty" form:"continue"`
}

// Validate checks query shape without parsing selectors or interpreting cursors.
// An omitted schema/limit is valid before WithDefaults; explicit invalid values
// are rejected rather than replaced with defaults.
func (q *ToolSearchQuery) Validate() error {
	if q == nil {
		return fmt.Errorf("$: tool search query is required")
	}

	var result *multierror.Error

	if q.ToolSetID != "" {
		if err := toolsetschema.ValidateID(q.ToolSetID); err != nil {
			result = multierror.Append(result, fmt.Errorf("$.toolSetId: %w", err))
		}
	}

	switch q.Schema {
	case "", ToolSchemaDetailNone, ToolSchemaDetailCompact, ToolSchemaDetailFull:
	default:
		result = multierror.Append(result, fmt.Errorf("$.schema: must be none, compact, or full"))
	}

	if q.Limit != nil && (*q.Limit < 1 || *q.Limit > MaxToolSearchLimit) {
		result = multierror.Append(result, fmt.Errorf("$.limit: must be between 1 and %d", MaxToolSearchLimit))
	}

	return result.ErrorOrNil()
}

// WithDefaults returns an independent query value with omission defaults applied.
// It detaches Limit even when supplied and preserves selector, query, and cursor
// text exactly. It does not validate the query or repair explicit invalid values.
func (q ToolSearchQuery) WithDefaults() ToolSearchQuery {
	if q.Schema == "" {
		q.Schema = ToolSchemaDetailNone
	}

	if q.Limit == nil {
		q.Limit = util.ToPtr(DefaultToolSearchLimit)
	} else {
		q.Limit = util.CloneValue(q.Limit)
	}

	return q
}
