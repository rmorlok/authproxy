package meta

import (
	"time"

	"github.com/rmorlok/authproxy/internal/schema/common"
)

// TypeMeta identifies the schema used to decode a resource or transport
// object. Embed it in a resource with yaml:",inline" so apiVersion and kind
// remain top-level fields in both JSON and YAML.
type TypeMeta struct {
	APIVersion APIVersion `json:"apiVersion" yaml:"apiVersion"`
	Kind       Kind       `json:"kind" yaml:"kind"`
}

// ObjectMeta contains identity and common lifecycle metadata shared by
// AuthProxy resources. Resource packages decide which fields are applicable
// and required for each operation.
type ObjectMeta struct {
	ID          string              `json:"id,omitempty" yaml:"id,omitempty"`
	Name        common.ResourceName `json:"name,omitempty" yaml:"name,omitempty"`
	Namespace   string              `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Generation  uint64              `json:"generation,omitempty" yaml:"generation,omitempty"`
	Labels      map[string]string   `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations map[string]string   `json:"annotations,omitempty" yaml:"annotations,omitempty"`
	CreatedAt   *time.Time          `json:"createdAt,omitempty" yaml:"createdAt,omitempty"`
	UpdatedAt   *time.Time          `json:"updatedAt,omitempty" yaml:"updatedAt,omitempty"`
}

// CloneObjectMeta returns a deep copy of metadata, including its mutable maps
// and timestamp pointers.
func CloneObjectMeta(value ObjectMeta) ObjectMeta {
	clone := value
	if value.Labels != nil {
		clone.Labels = make(map[string]string, len(value.Labels))
		for key, item := range value.Labels {
			clone.Labels[key] = item
		}
	}
	if value.Annotations != nil {
		clone.Annotations = make(map[string]string, len(value.Annotations))
		for key, item := range value.Annotations {
			clone.Annotations[key] = item
		}
	}
	if value.CreatedAt != nil {
		createdAt := *value.CreatedAt
		clone.CreatedAt = &createdAt
	}
	if value.UpdatedAt != nil {
		updatedAt := *value.UpdatedAt
		clone.UpdatedAt = &updatedAt
	}
	return clone
}

// ObjectMetaPatch represents a metadata merge patch. A nil field leaves the
// current value unchanged. A non-nil pointer to an empty labels or annotations
// map clears that map.
type ObjectMetaPatch struct {
	ID          *string              `json:"id,omitempty" yaml:"id,omitempty"`
	Name        *common.ResourceName `json:"name,omitempty" yaml:"name,omitempty"`
	Namespace   *string              `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Generation  *uint64              `json:"generation,omitempty" yaml:"generation,omitempty"`
	Labels      *map[string]string   `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations *map[string]string   `json:"annotations,omitempty" yaml:"annotations,omitempty"`
	CreatedAt   *time.Time           `json:"createdAt,omitempty" yaml:"createdAt,omitempty"`
	UpdatedAt   *time.Time           `json:"updatedAt,omitempty" yaml:"updatedAt,omitempty"`
}

// ObjectReference contains stable identity without copying another resource's
// spec or status. A reference identifies its target by ID or by the combination
// of namespace and name. Generation is populated only when it applies to the
// referenced kind.
type ObjectReference struct {
	APIVersion APIVersion          `json:"apiVersion" yaml:"apiVersion"`
	Kind       Kind                `json:"kind" yaml:"kind"`
	ID         string              `json:"id,omitempty" yaml:"id,omitempty"`
	Name       common.ResourceName `json:"name,omitempty" yaml:"name,omitempty"`
	Namespace  string              `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Generation uint64              `json:"generation,omitempty" yaml:"generation,omitempty"`
}

// HasID reports whether the reference identifies its target by immutable ID.
func (r ObjectReference) HasID() bool {
	return r.ID != ""
}

// HasNamespacedName reports whether the reference identifies its target by
// namespace and name.
func (r ObjectReference) HasNamespacedName() bool {
	return r.Namespace != "" && r.Name != ""
}

// ConditionStatus is the tri-state status of a resource condition.
type ConditionStatus string

const (
	ConditionTrue    ConditionStatus = "True"
	ConditionFalse   ConditionStatus = "False"
	ConditionUnknown ConditionStatus = "Unknown"
)

// Condition describes one server-observed aspect of resource state.
type Condition struct {
	Type               string          `json:"type" yaml:"type"`
	Status             ConditionStatus `json:"status" yaml:"status"`
	ObservedGeneration uint64          `json:"observedGeneration,omitempty" yaml:"observedGeneration,omitempty"`
	LastTransitionTime time.Time       `json:"lastTransitionTime" yaml:"lastTransitionTime"`
	Reason             string          `json:"reason,omitempty" yaml:"reason,omitempty"`
	Message            string          `json:"message,omitempty" yaml:"message,omitempty"`
}
