package config

import (
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	rlschema "github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
	"github.com/stretchr/testify/require"
)

func configuredRateLimitForValidation(id, name, namespace string) rlschema.RateLimit {
	return rlschema.RateLimit{
		TypeMeta: meta.NewTypeMeta(rlschema.RateLimitKind),
		Metadata: meta.ObjectMeta{
			ID:        id,
			Name:      common.ResourceName(name),
			Namespace: namespace,
		},
		Spec: rlschema.RateLimitSpec{
			Selector: rlschema.Selector{},
			Bucket:   rlschema.Bucket{},
			Algorithm: rlschema.Algorithm{
				TokenBucket: &rlschema.TokenBucket{Capacity: 10, RefillRate: 1},
			},
		},
	}
}

func TestRateLimitsValidate(t *testing.T) {
	valid := configuredRateLimitForValidation("", "tenant-default", "root.acme")
	require.NoError(t, (&RateLimits{LoadFromList: []rlschema.RateLimit{valid}}).Validate(nil))

	missingIdentity := configuredRateLimitForValidation("", "", "root.acme")
	require.ErrorContains(t, (&RateLimits{LoadFromList: []rlschema.RateLimit{missingIdentity}}).Validate(nil), "id or name is required")

	duplicateName := configuredRateLimitForValidation("rl_test0000000000001", "tenant-default", "root.acme")
	require.ErrorContains(t, (&RateLimits{LoadFromList: []rlschema.RateLimit{valid, duplicateName}}).Validate(nil), "duplicates item 0")

	duplicateID := configuredRateLimitForValidation("rl_test0000000000001", "other", "root.acme")
	require.ErrorContains(t, (&RateLimits{LoadFromList: []rlschema.RateLimit{duplicateName, duplicateID}}).Validate(nil), "duplicates item 0")
}
