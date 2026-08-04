package api

import (
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
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

// NotificationJson is the actor-specific API projection of a notification.
//
//	@Description	Actor-visible notification
type NotificationJson struct {
	Id           apid.ID           `json:"id" yaml:"id" swaggertype:"string" example:"ntf_test550e8400abcde"`
	Key          string            `json:"key" yaml:"key"`
	Level        NotificationLevel `json:"level" yaml:"level" swaggertype:"string" example:"warning"`
	State        NotificationState `json:"state" yaml:"state" swaggertype:"string" example:"active"`
	ResourceType string            `json:"resourceType" yaml:"resourceType" example:"connection"`
	ResourceId   apid.ID           `json:"resourceId" yaml:"resourceId" swaggertype:"string" example:"cxn_test550e8400abcde"`
	Namespace    string            `json:"namespace" yaml:"namespace" example:"root.acme"`
	Title        string            `json:"title" yaml:"title"`
	Message      string            `json:"message" yaml:"message"`
	ActionUrl    string            `json:"actionUrl,omitempty" yaml:"actionUrl,omitempty"`
	CanAction    bool              `json:"canAction" yaml:"canAction"`
	Viewed       bool              `json:"viewed" yaml:"viewed"`
	Metadata     map[string]any    `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	CreatedAt    time.Time         `json:"createdAt" yaml:"createdAt"`
	UpdatedAt    time.Time         `json:"updatedAt" yaml:"updatedAt"`
	ResolvedAt   *time.Time        `json:"resolvedAt,omitempty" yaml:"resolvedAt,omitempty"`
}

type ListNotificationsResponseJson struct {
	Items  []NotificationJson `json:"items" yaml:"items"`
	Cursor string             `json:"cursor,omitempty" yaml:"cursor,omitempty"`
}

type MarkNotificationsViewedRequestJson struct {
	Ids []apid.ID `json:"ids" yaml:"ids" swaggertype:"array,string" example:"ntf_test550e8400abcde"`
}

// NotificationUpsertJson is an internal service shape used by migration hooks
// and core code when creating deterministic notifications.
type NotificationUpsertJson struct {
	Key               string               `json:"key" yaml:"key"`
	Level             NotificationLevel    `json:"level" yaml:"level"`
	ResourceType      string               `json:"resourceType" yaml:"resourceType"`
	ResourceId        apid.ID              `json:"resourceId" yaml:"resourceId"`
	Namespace         string               `json:"namespace" yaml:"namespace"`
	Title             string               `json:"title" yaml:"title"`
	Message           string               `json:"message" yaml:"message"`
	ActionUrl         string               `json:"actionUrl,omitempty" yaml:"actionUrl,omitempty"`
	ViewPermissions   []aschema.Permission `json:"viewPermissions,omitempty" yaml:"viewPermissions,omitempty"`
	ActionPermissions []aschema.Permission `json:"actionPermissions,omitempty" yaml:"actionPermissions,omitempty"`
	Metadata          map[string]any       `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}
