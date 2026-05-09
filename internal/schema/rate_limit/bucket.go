package rate_limit

import (
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
)

// Reserved bucket-dimension names that resolve to request-context fields
// rather than to a request label.
const (
	DimensionActor            = "actor"
	DimensionConnection       = "connection"
	DimensionConnector        = "connector"
	DimensionConnectorVersion = "connector_version"
	DimensionNamespace        = "namespace"
	DimensionMethod           = "method"
)

// LabelDimensionPrefix marks a dimension whose value comes from the
// per-request label snapshot. The text after the prefix is the label key.
const LabelDimensionPrefix = "labels/"

var reservedDimensions = map[string]bool{
	DimensionActor:            true,
	DimensionConnection:       true,
	DimensionConnector:        true,
	DimensionConnectorVersion: true,
	DimensionNamespace:        true,
	DimensionMethod:           true,
}

// IsReservedDimension reports whether name refers to a request-context field.
func IsReservedDimension(name string) bool {
	return reservedDimensions[name]
}

// Bucket projects matched requests into independent counters. An empty
// Dimensions list means a single global bucket per rule.
type Bucket struct {
	Dimensions []string `json:"dimensions,omitempty" yaml:"dimensions,omitempty"`
}

// Validate ensures every dimension is either a reserved name or a
// well-formed labels/<key> reference.
func (b *Bucket) Validate(vc *common.ValidationContext) error {
	result := &multierror.Error{}

	seen := make(map[string]int, len(b.Dimensions))
	for i, d := range b.Dimensions {
		if d == "" {
			result = multierror.Append(result, vc.PushField("dimensions").PushIndex(i).NewError("must not be empty"))
			continue
		}
		if prev, ok := seen[d]; ok {
			result = multierror.Append(result, vc.PushField("dimensions").PushIndex(i).NewErrorf("duplicate dimension %q (also at index %d)", d, prev))
			continue
		}
		seen[d] = i

		if IsReservedDimension(d) {
			continue
		}

		if !strings.HasPrefix(d, LabelDimensionPrefix) {
			result = multierror.Append(result, vc.PushField("dimensions").PushIndex(i).NewErrorf("must be a reserved name or %q-prefixed label reference, got %q", LabelDimensionPrefix, d))
			continue
		}

		// Lightweight key check; the runtime evaluator validates against
		// real labels so any key that survives this gate is well-defined
		// either as a user label or as a no-match.
		key := strings.TrimPrefix(d, LabelDimensionPrefix)
		if key == "" {
			result = multierror.Append(result, vc.PushField("dimensions").PushIndex(i).NewErrorf("missing label key after %q in %q", LabelDimensionPrefix, d))
		}
	}

	return result.ErrorOrNil()
}
