package v1alpha1

import (
	"strings"

	"github.com/hashicorp/go-multierror"
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

// ListKind returns the list kind corresponding to itemKind.
func ListKind(itemKind meta.Kind) meta.Kind {
	if strings.HasSuffix(string(itemKind), "List") {
		return itemKind
	}
	return meta.Kind(string(itemKind) + "List")
}

// NewResourceList creates a list response and normalizes nil items to an empty array.
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

// NewActionRequest creates an action request without response status.
func NewActionRequest[TSpec any](kind meta.Kind, target meta.ObjectReference, spec TSpec) Action[TSpec, struct{}] {
	return Action[TSpec, struct{}]{
		TypeMeta: meta.NewTypeMeta(kind),
		Metadata: ActionMeta{Target: target},
		Spec:     spec,
	}
}

// NewActionResponse creates an action response containing its status.
func NewActionResponse[TSpec any, TStatus any](kind meta.Kind, target meta.ObjectReference, spec TSpec, status TStatus) Action[TSpec, TStatus] {
	return Action[TSpec, TStatus]{
		TypeMeta: meta.NewTypeMeta(kind),
		Metadata: ActionMeta{Target: target},
		Spec:     spec,
		Status:   &status,
	}
}

// Validate verifies the list envelope for resources of itemKind.
func (r *ResourceList[T]) Validate(itemKind meta.Kind) error {
	vc := &common.ValidationContext{Path: "$"}
	var result *multierror.Error
	expectedKind := ListKind(itemKind)
	if err := meta.ValidateTypeMeta(r.TypeMeta, APIVersion, expectedKind, vc); err != nil {
		result = multierror.Append(result, err)
	}
	if err := r.Metadata.Validate(vc.PushField("metadata")); err != nil {
		result = multierror.Append(result, err)
	}
	return result.ErrorOrNil()
}

// Validate verifies list pagination metadata.
func (m *ListMeta) Validate(vc *common.ValidationContext) error {
	if vc == nil {
		vc = &common.ValidationContext{Path: "$.metadata"}
	}
	if m.RemainingItemCount != nil && *m.RemainingItemCount < 0 {
		return vc.NewErrorForField("remainingItemCount", "must not be negative")
	}
	return nil
}

// Validate verifies the action envelope for expectedKind.
func (a *Action[TSpec, TStatus]) Validate(expectedKind meta.Kind) error {
	return a.ValidateResponse(expectedKind)
}

// ValidateRequest verifies an action received from a client. Action status is
// server-owned and therefore must not be present in request bodies.
func (a *Action[TSpec, TStatus]) ValidateRequest(expectedKind meta.Kind) error {
	return a.validateFor(expectedKind, meta.ValidationModeCreate)
}

// ValidateResponse verifies an action returned by the server. Response status
// is allowed because it contains the observed result of the action.
func (a *Action[TSpec, TStatus]) ValidateResponse(expectedKind meta.Kind) error {
	return a.validateFor(expectedKind, meta.ValidationModeResponse)
}

func (a *Action[TSpec, TStatus]) validateFor(expectedKind meta.Kind, mode meta.ValidationMode) error {
	vc := &common.ValidationContext{Path: "$"}
	if a == nil {
		return vc.NewError("action is required")
	}
	if strings.HasSuffix(string(expectedKind), "List") {
		return vc.NewErrorfForField("kind", "action kind %q must not be a list kind", expectedKind)
	}

	var result *multierror.Error
	if err := meta.ValidateTypeMeta(a.TypeMeta, APIVersion, expectedKind, vc); err != nil {
		result = multierror.Append(result, err)
	}
	if err := a.Metadata.Validate(vc.PushField("metadata")); err != nil {
		result = multierror.Append(result, err)
	}
	if err := meta.ValidateStatus(a.Status, mode, vc); err != nil {
		result = multierror.Append(result, err)
	}
	return result.ErrorOrNil()
}

// Validate verifies action target metadata.
func (m *ActionMeta) Validate(vc *common.ValidationContext) error {
	if vc == nil {
		vc = &common.ValidationContext{Path: "$.metadata"}
	}
	return meta.ValidateObjectReference(m.Target, vc.PushField("target"))
}
