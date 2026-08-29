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
// spec or status. ID, name, namespace, and generation are populated only when
// they apply to the referenced kind.
type ObjectReference struct {
	APIVersion APIVersion          `json:"apiVersion" yaml:"apiVersion"`
	Kind       Kind                `json:"kind" yaml:"kind"`
	ID         string              `json:"id,omitempty" yaml:"id,omitempty"`
	Name       common.ResourceName `json:"name,omitempty" yaml:"name,omitempty"`
	Namespace  string              `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Generation uint64              `json:"generation,omitempty" yaml:"generation,omitempty"`
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
