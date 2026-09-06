package api

import (
	"fmt"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apid"
	apiv1alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
)

const (
	NotificationKind                meta.Kind = "Notification"
	NotificationViewActionKind      meta.Kind = "NotificationView"
	NotificationBatchViewActionKind meta.Kind = "NotificationBatchView"
)

type NotificationLevel string

const (
	NotificationLevelInfo    NotificationLevel = "info"
	NotificationLevelWarning NotificationLevel = "warning"
	NotificationLevelError   NotificationLevel = "error"
)

type NotificationState string

const (
	NotificationStateActive   NotificationState = "active"
	NotificationStateResolved NotificationState = "resolved"
)

// NotificationJson is an actor-specific, read-only projection of a durable
// notification. The underlying notification is shared, while Status.Viewed
// and Status.Action are calculated for the authenticated actor.
//
//	@Description	Actor-visible, resource-shaped notification projection
type NotificationJson struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      meta.ObjectMeta        `json:"metadata" yaml:"metadata"`
	Spec          NotificationSpecJson   `json:"spec" yaml:"spec"`
	Status        NotificationStatusJson `json:"status" yaml:"status"`
}

// NotificationSpecJson contains the notification content and the typed
// reference to the resource whose condition produced it.
type NotificationSpecJson struct {
	Key         string               `json:"key" yaml:"key"`
	Level       NotificationLevel    `json:"level" yaml:"level" swaggertype:"string" example:"warning"`
	ResourceRef meta.ObjectReference `json:"resourceRef" yaml:"resourceRef"`
	Title       string               `json:"title" yaml:"title"`
	Message     string               `json:"message" yaml:"message"`
	Context     map[string]any       `json:"context,omitempty" yaml:"context,omitempty"`
}

// NotificationStatusJson combines the shared notification lifecycle with
// observations specific to the authenticated actor.
type NotificationStatusJson struct {
	State      NotificationState             `json:"state" yaml:"state" swaggertype:"string" example:"active"`
	Viewed     bool                          `json:"viewed" yaml:"viewed"`
	Action     *NotificationActionStatusJson `json:"action,omitempty" yaml:"action,omitempty"`
	ResolvedAt *time.Time                    `json:"resolvedAt,omitempty" yaml:"resolvedAt,omitempty"`
}

// NotificationActionStatusJson is present only when the authenticated actor
// is authorized to perform the notification's suggested action.
type NotificationActionStatusJson struct {
	URL string `json:"url" yaml:"url"`
}

// ListNotificationsResponseJson is the Kubernetes-style list of actor-visible
// Notification projections.
type ListNotificationsResponseJson struct {
	apiv1alpha1.ResourceList[NotificationJson] `json:",inline" yaml:",inline"`
}

func NewListNotificationsResponseJson(
	items []NotificationJson,
	continueToken string,
) ListNotificationsResponseJson {
	return ListNotificationsResponseJson{ResourceList: apiv1alpha1.NewResourceList(
		NotificationKind,
		items,
		apiv1alpha1.ListMeta{Continue: continueToken},
	)}
}

type NotificationViewSpec struct{}

type NotificationViewStatus struct {
	Viewed bool `json:"viewed" yaml:"viewed"`
}

type NotificationViewAction struct {
	apiv1alpha1.Action[NotificationViewSpec, NotificationViewStatus] `json:",inline" yaml:",inline"`
}

func NewNotificationViewRequest(target meta.ObjectReference) NotificationViewAction {
	return NotificationViewAction{Action: apiv1alpha1.Action[NotificationViewSpec, NotificationViewStatus]{
		TypeMeta: meta.NewTypeMeta(NotificationViewActionKind),
		Metadata: apiv1alpha1.ActionMeta{Target: target},
		Spec:     NotificationViewSpec{},
	}}
}

func (a *NotificationViewAction) ValidateRequest(expectedKind meta.Kind) error {
	if err := a.Action.ValidateRequest(expectedKind); err != nil {
		return err
	}
	return validateNotificationReference(a.Metadata.Target, &common.ValidationContext{Path: "$.metadata.target"})
}

func (a *NotificationViewAction) ValidateResponse(expectedKind meta.Kind) error {
	if err := a.Action.ValidateResponse(expectedKind); err != nil {
		return err
	}
	if err := validateNotificationReference(a.Metadata.Target, &common.ValidationContext{Path: "$.metadata.target"}); err != nil {
		return err
	}
	if a.Status == nil {
		return fmt.Errorf("$.status: is required")
	}
	if !a.Status.Viewed {
		return fmt.Errorf("$.status.viewed: must be true")
	}
	return nil
}

func NewNotificationViewResponse(target meta.ObjectReference) NotificationViewAction {
	return NotificationViewAction{Action: apiv1alpha1.NewActionResponse(
		NotificationViewActionKind,
		target,
		NotificationViewSpec{},
		NotificationViewStatus{Viewed: true},
	)}
}

// NotificationBatchViewMetadata identifies every Notification projection to
// mark viewed. Targets are references because the batch has no single action
// target suitable for the shared action transport.
type NotificationBatchViewMetadata struct {
	Targets []meta.ObjectReference `json:"targets" yaml:"targets"`
}

type NotificationBatchViewSpec struct{}

type NotificationBatchViewStatus struct {
	ViewedCount int `json:"viewedCount" yaml:"viewedCount"`
}

type NotificationBatchViewAction struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      *NotificationBatchViewMetadata `json:"metadata" yaml:"metadata"`
	Spec          *NotificationBatchViewSpec     `json:"spec" yaml:"spec"`
	Status        *NotificationBatchViewStatus   `json:"status,omitempty" yaml:"status,omitempty"`
}

func (a *NotificationBatchViewAction) ValidateRequest(expectedKind meta.Kind) error {
	return a.validate(expectedKind, false)
}

func (a *NotificationBatchViewAction) ValidateResponse(expectedKind meta.Kind) error {
	return a.validate(expectedKind, true)
}

func (a *NotificationBatchViewAction) validate(expectedKind meta.Kind, response bool) error {
	if a == nil {
		return fmt.Errorf("notification batch view action is required")
	}
	vc := &common.ValidationContext{Path: "$"}
	var result *multierror.Error
	if err := meta.ValidateTypeMeta(a.TypeMeta, meta.APIVersionV1Alpha1, expectedKind, vc); err != nil {
		result = multierror.Append(result, err)
	}
	if a.Metadata == nil {
		result = multierror.Append(result, vc.NewErrorForField("metadata", "is required"))
	} else if len(a.Metadata.Targets) == 0 {
		result = multierror.Append(result, vc.NewErrorForField("metadata.targets", "must not be empty"))
	} else {
		seen := make(map[string]struct{}, len(a.Metadata.Targets))
		for i, target := range a.Metadata.Targets {
			if err := validateNotificationReference(target, &common.ValidationContext{Path: fmt.Sprintf("$.metadata.targets[%d]", i)}); err != nil {
				result = multierror.Append(result, err)
			}
			if _, found := seen[target.ID]; found {
				result = multierror.Append(result, vc.NewErrorfForField("metadata.targets", "contains duplicate id %q", target.ID))
			}
			seen[target.ID] = struct{}{}
		}
	}
	if a.Spec == nil {
		result = multierror.Append(result, vc.NewErrorForField("spec", "is required"))
	}
	if response {
		if a.Status == nil {
			result = multierror.Append(result, vc.NewErrorForField("status", "is required"))
		} else if a.Metadata != nil && a.Status.ViewedCount != len(a.Metadata.Targets) {
			result = multierror.Append(result, vc.NewErrorForField("status.viewedCount", "must match the number of targets"))
		}
	} else if a.Status != nil {
		result = multierror.Append(result, vc.NewErrorForField("status", "is server-owned"))
	}
	return result.ErrorOrNil()
}

func NewNotificationBatchViewResponse(
	targets []meta.ObjectReference,
) NotificationBatchViewAction {
	return NotificationBatchViewAction{
		TypeMeta: meta.NewTypeMeta(NotificationBatchViewActionKind),
		Metadata: &NotificationBatchViewMetadata{Targets: targets},
		Spec:     &NotificationBatchViewSpec{},
		Status:   &NotificationBatchViewStatus{ViewedCount: len(targets)},
	}
}

func NewNotificationReference(id apid.ID) meta.ObjectReference {
	return meta.ObjectReference{
		APIVersion: meta.APIVersionV1Alpha1,
		Kind:       NotificationKind,
		ID:         id.String(),
	}
}

func validateNotificationReference(ref meta.ObjectReference, vc *common.ValidationContext) error {
	var result *multierror.Error
	if err := meta.ValidateObjectReferenceWithOptions(ref, meta.ObjectReferenceValidationOptions{
		ExpectedAPIVersion: meta.APIVersionV1Alpha1,
		ExpectedKind:       NotificationKind,
		IDValidator: func(value string) error {
			id, err := apid.Parse(value)
			if err != nil {
				return err
			}
			return id.ValidatePrefix(apid.PrefixNotification)
		},
	}, vc); err != nil {
		result = multierror.Append(result, err)
	}
	if ref.ID == "" {
		result = multierror.Append(result, vc.NewErrorForField("id", "is required for Notification references"))
	}
	if ref.Name != "" || ref.Namespace != "" {
		result = multierror.Append(result, vc.NewError("Notification references support id only"))
	}
	if ref.Generation != 0 {
		result = multierror.Append(result, vc.NewErrorForField("generation", "does not apply to Notification references"))
	}
	return result.ErrorOrNil()
}
