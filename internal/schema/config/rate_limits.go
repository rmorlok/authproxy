package config

import (
	"fmt"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	rlschema "github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
)

// RateLimits configures canonical RateLimit resources reconciled at startup.
type RateLimits struct {
	LoadFromList []rlschema.RateLimit `json:"loadFromList,omitempty" yaml:"loadFromList,omitempty"`
}

func (r *RateLimits) GetRateLimits() []rlschema.RateLimit {
	if r == nil {
		return nil
	}
	return r.LoadFromList
}

func (r *RateLimits) Validate(vc *common.ValidationContext) error {
	if r == nil {
		return nil
	}
	if vc == nil {
		vc = &common.ValidationContext{Path: "$"}
	}
	var result *multierror.Error
	ids := map[string]int{}
	names := map[string]int{}
	for i := range r.LoadFromList {
		resource := &r.LoadFromList[i]
		itemVC := vc.PushField("loadFromList").PushIndex(i)
		if err := resource.ValidateFor(meta.ValidationModeConfig, itemVC); err != nil {
			result = multierror.Append(result, err)
		}
		if resource.Metadata.ID == "" && resource.Metadata.Name == "" {
			result = multierror.Append(result, itemVC.NewErrorForField("metadata", "id or name is required for configuration reconciliation"))
		}
		if id := resource.Metadata.ID; id != "" {
			if previous, exists := ids[id]; exists {
				result = multierror.Append(result, itemVC.NewErrorfForField("metadata.id", "duplicates item %d", previous))
			} else {
				ids[id] = i
			}
		}
		if name := resource.Metadata.Name; name != "" {
			key := resource.Metadata.Namespace + "/" + string(name)
			if previous, exists := names[key]; exists {
				result = multierror.Append(result, itemVC.NewErrorfForField("metadata.name", "duplicates item %d in namespace %q", previous, resource.Metadata.Namespace))
			} else {
				names[key] = i
			}
		}
	}
	return result.ErrorOrNil()
}

func (r *RateLimits) String() string {
	return fmt.Sprintf("%d configured rate limits", len(r.GetRateLimits()))
}
