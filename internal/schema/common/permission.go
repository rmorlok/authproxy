package common

import (
	"errors"
	"slices"

	"github.com/hashicorp/go-multierror"
)

// Wildcard constant for resources and verbs that matches any value.
const PermissionWildcard = "*"

// NamespaceWildcardSuffix is appended to a namespace to indicate all child namespaces are included.
// For example, "root.**" matches "root", "root.foo", "root.foo.bar", etc.
const NamespaceWildcardSuffix = ".**"

type Permission struct {
	Namespace   string   `json:"namespace" yaml:"namespace"`
	Resources   []string `json:"resources" yaml:"resources"`
	ResourceIds []string `json:"resource_ids" yaml:"resource_ids"`
	Verbs       []string `json:"verbs" yaml:"verbs"`
}

func (p Permission) Equal(other Permission) bool {
	return p.Namespace == other.Namespace &&
		slices.Equal(p.Resources, other.Resources) &&
		slices.Equal(p.ResourceIds, other.ResourceIds) &&
		slices.Equal(p.Verbs, other.Verbs)
}

func (p Permission) Validate() error {
	result := &multierror.Error{}

	if p.Namespace == "" {
		result = multierror.Append(result, errors.New("permission namespace is required"))
	}

	for _, resourceId := range p.ResourceIds {
		if resourceId == "" {
			result = multierror.Append(result, errors.New("permission resource id is required"))
		}
	}

	if len(p.Resources) == 0 {
		result = multierror.Append(result, errors.New("at least one permission resource is required"))
	}

	for _, resource := range p.Resources {
		if resource == "" {
			result = multierror.Append(result, errors.New("permission resource is required"))
		}
	}

	if len(p.Verbs) == 0 {
		result = multierror.Append(result, errors.New("at least one permissions verb is required"))
	}

	for _, verb := range p.Verbs {
		if verb == "" {
			result = multierror.Append(result, errors.New("permission verb is required"))
		}
	}

	return result.ErrorOrNil()
}
