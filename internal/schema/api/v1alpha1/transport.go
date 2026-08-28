package v1alpha1

import (
	"fmt"
	"strings"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
)

// ListMeta contains pagination and snapshot information for a resource list.
// Continue is an opaque token supplied to the next request.
type ListMeta struct {
	ResourceVersion    string `json:"resourceVersion,omitempty" yaml:"resourceVersion,omitempty"`
	Continue           string `json:"continue,omitempty" yaml:"continue,omitempty"`
	RemainingItemCount *int64 `json:"remainingItemCount,omitempty" yaml:"remainingItemCount,omitempty"`
}

// ResourceList is the common Kubernetes-style list transport. Item packages
// own T; the API package owns this envelope and pagination metadata.
type ResourceList[T any] struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      ListMeta `json:"metadata" yaml:"metadata"`
	Items         []T      `json:"items" yaml:"items"`
}

// ActionMeta identifies the resource targeted by an imperative action.
type ActionMeta struct {
	Target meta.ObjectReference `json:"target" yaml:"target"`
}

// Action is a typed, non-durable imperative transport. Status is absent on a
// request and may be populated by synchronous or asynchronous results.
type Action[TSpec any, TStatus any] struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      ActionMeta `json:"metadata" yaml:"metadata"`
	Spec          TSpec      `json:"spec" yaml:"spec"`
	Status        *TStatus   `json:"status,omitempty" yaml:"status,omitempty"`
}

func ListKind(itemKind meta.Kind) meta.Kind {
	if strings.HasSuffix(string(itemKind), "List") {
		return itemKind
	}
	return meta.Kind(string(itemKind) + "List")
}

func NewResourceList[T any](itemKind meta.Kind, items []T, listMeta ListMeta) ResourceList[T] {
	if items == nil {
		items = make([]T, 0)
	}
	return ResourceList[T]{
		TypeMeta: meta.NewTypeMeta(ListKind(itemKind)),
		Metadata: listMeta,
		Items:    items,
	}
}

func NewActionRequest[TSpec any](kind meta.Kind, target meta.ObjectReference, spec TSpec) Action[TSpec, struct{}] {
	return Action[TSpec, struct{}]{
		TypeMeta: meta.NewTypeMeta(kind),
		Metadata: ActionMeta{Target: target},
		Spec:     spec,
	}
}

func NewActionResponse[TSpec any, TStatus any](kind meta.Kind, target meta.ObjectReference, spec TSpec, status TStatus) Action[TSpec, TStatus] {
	return Action[TSpec, TStatus]{
		TypeMeta: meta.NewTypeMeta(kind),
		Metadata: ActionMeta{Target: target},
		Spec:     spec,
		Status:   &status,
	}
}

func ValidateResourceListType(typeMeta meta.TypeMeta, itemKind meta.Kind) error {
	expectedKind := ListKind(itemKind)
	if err := meta.ValidateTypeMeta(typeMeta, APIVersion, expectedKind, nil); err != nil {
		return err
	}
	return nil
}

func ValidateListMeta(value ListMeta) error {
	if value.RemainingItemCount != nil && *value.RemainingItemCount < 0 {
		return (&common.ValidationContext{Path: "$.metadata"}).NewErrorForField("remainingItemCount", "must not be negative")
	}
	return nil
}

func ValidateActionType(typeMeta meta.TypeMeta, expectedKind meta.Kind) error {
	if strings.HasSuffix(string(expectedKind), "List") {
		return fmt.Errorf("action kind %q must not be a list kind", expectedKind)
	}
	return meta.ValidateTypeMeta(typeMeta, APIVersion, expectedKind, nil)
}

func ValidateActionMeta(value ActionMeta) error {
	return meta.ValidateObjectReference(value.Target, &common.ValidationContext{Path: "$.metadata.target"})
}
