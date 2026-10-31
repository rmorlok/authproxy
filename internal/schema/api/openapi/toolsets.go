package openapi

import (
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
)

// ToolSetConnectionSelectorJson documents the logical connection selector
// shared by every generation of a ToolSet.
type ToolSetConnectionSelectorJson struct {
	Namespace   string            `json:"namespace,omitempty" example:"root.acme.**"`
	MatchLabels map[string]string `json:"matchLabels" binding:"required"`
}

// ToolSetReleaseJson documents a generation's publication intent.
type ToolSetReleaseJson struct {
	DesiredState string `json:"desiredState,omitempty" enums:"draft,primary" example:"draft"`
}

// ToolSetSpecJson documents a ToolSet generation's desired state. The definition
// is kept opaque to swaggo; the canonical contract is the ToolSet resource JSON
// schema in internal/schema/resources/toolsets.
type ToolSetSpecJson struct {
	ConnectionSelector ToolSetConnectionSelectorJson `json:"connectionSelector" binding:"required"`
	Release            *ToolSetReleaseJson           `json:"release,omitempty"`
	Definition         map[string]interface{}        `json:"definition" binding:"required" swaggertype:"object"`
}

// ToolSetReleaseStatusJson documents a generation's observed release state.
type ToolSetReleaseStatusJson struct {
	State string `json:"state" binding:"required" enums:"draft,primary,active,archived"`
}

// ToolSetStatusJson documents server-owned ToolSet generation status.
type ToolSetStatusJson struct {
	Release ToolSetReleaseStatusJson `json:"release" binding:"required"`
}

// ToolSetJson documents the management ToolSet resource.
//
//	@Description	Kubernetes-style ToolSet resource projected at one generation
type ToolSetJson struct {
	APIVersion string             `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string             `json:"kind" binding:"required" enums:"ToolSet" example:"ToolSet"`
	Metadata   meta.ObjectMeta    `json:"metadata" binding:"required"`
	Spec       ToolSetSpecJson    `json:"spec" binding:"required"`
	Status     *ToolSetStatusJson `json:"status,omitempty"`
}

// ToolSetSpecPatchJson documents a partial spec update. A supplied selector or
// definition replaces the whole field; release merges only desiredState.
type ToolSetSpecPatchJson struct {
	ConnectionSelector *ToolSetConnectionSelectorJson `json:"connectionSelector,omitempty"`
	Release            *ToolSetReleaseJson            `json:"release,omitempty"`
	Definition         map[string]interface{}         `json:"definition,omitempty" swaggertype:"object"`
}

// ToolSetPatchJson documents ToolSet and ToolSet generation PATCH bodies.
type ToolSetPatchJson struct {
	APIVersion string                `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string                `json:"kind" binding:"required" enums:"ToolSet" example:"ToolSet"`
	Metadata   *meta.ObjectMetaPatch `json:"metadata" binding:"required"`
	Spec       *ToolSetSpecPatchJson `json:"spec" binding:"required"`
}

// ListToolSetsResponseJson documents a paginated ToolSet list.
//
//	@Description	Paginated list of ToolSets or of one ToolSet's generations
type ListToolSetsResponseJson struct {
	ResourceListJson
	Items []ToolSetJson `json:"items" binding:"required"`
}

// ConnectionToolSetBindingConditionJson documents one observed binding condition.
// Status is a plain string so swaggo does not resolve the shared enum
// nondeterministically.
type ConnectionToolSetBindingConditionJson struct {
	Type               string `json:"type" binding:"required" example:"Installed"`
	Status             string `json:"status" binding:"required" enums:"True,False,Unknown"`
	ObservedGeneration uint64 `json:"observedGeneration,omitempty" example:"3"`
	LastTransitionTime string `json:"lastTransitionTime" binding:"required" format:"date-time"`
	Reason             string `json:"reason,omitempty" example:"InventoryInstalled"`
	Message            string `json:"message,omitempty"`
}

// ConnectionToolSetBindingSpecJson documents the connection and logical ToolSet
// that identify an assignment.
type ConnectionToolSetBindingSpecJson struct {
	ConnectionRef meta.ObjectReference `json:"connectionRef" binding:"required"`
	ToolSetRef    meta.ObjectReference `json:"toolSetRef" binding:"required"`
}

// ConnectionToolSetBindingStatusJson documents the installed inventory's state.
type ConnectionToolSetBindingStatusJson struct {
	AppliedGeneration *uint64                                 `json:"appliedGeneration,omitempty" example:"3"`
	Conditions        []ConnectionToolSetBindingConditionJson `json:"conditions,omitempty"`
}

// ConnectionToolSetBindingJson documents one read-only binding projection.
//
//	@Description	One ToolSet's assignment to a connection and its installation status
type ConnectionToolSetBindingJson struct {
	APIVersion string                             `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string                             `json:"kind" binding:"required" enums:"ConnectionToolSetBinding" example:"ConnectionToolSetBinding"`
	Metadata   meta.ObjectMeta                    `json:"metadata" binding:"required"`
	Spec       ConnectionToolSetBindingSpecJson   `json:"spec" binding:"required"`
	Status     ConnectionToolSetBindingStatusJson `json:"status" binding:"required"`
}

// ConnectionToolSetBindingListJson documents a paginated list of bindings.
//
//	@Description	Paginated ToolSet assignments for one connection
type ConnectionToolSetBindingListJson struct {
	ResourceListJson
	Items []ConnectionToolSetBindingJson `json:"items" binding:"required"`
}
