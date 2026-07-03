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
	ResourceType string            `json:"resource_type" yaml:"resource_type" example:"connection"`
	ResourceId   apid.ID           `json:"resource_id" yaml:"resource_id" swaggertype:"string" example:"cxn_test550e8400abcde"`
	Namespace    string            `json:"namespace" yaml:"namespace" example:"root.acme"`
	Title        string            `json:"title" yaml:"title"`
	Message      string            `json:"message" yaml:"message"`
	ActionUrl    string            `json:"action_url,omitempty" yaml:"action_url,omitempty"`
	CanAction    bool              `json:"can_action" yaml:"can_action"`
	Viewed       bool              `json:"viewed" yaml:"viewed"`
	Metadata     map[string]any    `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	CreatedAt    time.Time         `json:"created_at" yaml:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at" yaml:"updated_at"`
	ResolvedAt   *time.Time        `json:"resolved_at,omitempty" yaml:"resolved_at,omitempty"`
}

type ListNotificationsResponseJson struct {
	Items  []NotificationJson `json:"items" yaml:"items"`
	Cursor string             `json:"cursor,omitempty" yaml:"cursor,omitempty"`
}

// NotificationUpsertJson is an internal service shape used by migration hooks
// and core code when creating deterministic notifications.
type NotificationUpsertJson struct {
	Key               string               `json:"key" yaml:"key"`
	Level             NotificationLevel    `json:"level" yaml:"level"`
	ResourceType      string               `json:"resource_type" yaml:"resource_type"`
	ResourceId        apid.ID              `json:"resource_id" yaml:"resource_id"`
	Namespace         string               `json:"namespace" yaml:"namespace"`
	Title             string               `json:"title" yaml:"title"`
	Message           string               `json:"message" yaml:"message"`
	ActionUrl         string               `json:"action_url,omitempty" yaml:"action_url,omitempty"`
	ViewPermissions   []aschema.Permission `json:"view_permissions,omitempty" yaml:"view_permissions,omitempty"`
	ActionPermissions []aschema.Permission `json:"action_permissions,omitempty" yaml:"action_permissions,omitempty"`
	Source            string               `json:"source,omitempty" yaml:"source,omitempty"`
	Metadata          map[string]any       `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}
