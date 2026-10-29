package openapi

import (
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	toolschema "github.com/rmorlok/authproxy/internal/schema/resources/tools"
)

// ToolSpecJson documents a standalone Tool's desired state. JSON Schemas and
// the HTTP executor are kept opaque to swaggo; the canonical contract is the
// Tool resource JSON schema in internal/schema/resources/tools.
type ToolSpecJson struct {
	ConnectionRef meta.ObjectReference        `json:"connectionRef" binding:"required"`
	Description   string                      `json:"description" binding:"required" example:"List calendars available to this account."`
	Verbs         []string                    `json:"verbs" binding:"required" example:"tool:calendar.list"`
	InputSchema   map[string]interface{}      `json:"inputSchema" binding:"required" swaggertype:"object"`
	OutputSchema  map[string]interface{}      `json:"outputSchema,omitempty" swaggertype:"object"`
	Hints         *toolschema.BehavioralHints `json:"hints,omitempty"`
	Limits        *toolschema.ExecutionLimits `json:"limits,omitempty"`
	ProxyHTTP     map[string]interface{}      `json:"proxyHttp,omitempty" swaggertype:"object"`
	Javascript    *string                     `json:"javascript,omitempty"`
}

// ToolConditionJson documents one observed readiness condition. Status is a
// plain string so swaggo does not resolve the shared enum nondeterministically.
type ToolConditionJson struct {
	Type               string `json:"type" binding:"required"`
	Status             string `json:"status" binding:"required" enums:"True,False,Unknown"`
	ObservedRevision   uint64 `json:"observedRevision,omitempty"`
	LastTransitionTime string `json:"lastTransitionTime" binding:"required" format:"date-time"`
	Reason             string `json:"reason,omitempty"`
	Message            string `json:"message,omitempty"`
}

// ToolStatusJson documents server-owned Tool status: the active revision,
// readiness conditions, and, for generated Tools, the owning ToolSet.
type ToolStatusJson struct {
	Revision   uint64                    `json:"revision" binding:"required" example:"1"`
	Conditions []ToolConditionJson       `json:"conditions,omitempty"`
	ManagedBy  *toolschema.ToolManagedBy `json:"managedBy,omitempty"`
}

// ToolJson documents the management Tool resource.
//
//	@Description	Kubernetes-style connection-bound Tool resource
type ToolJson struct {
	APIVersion string          `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string          `json:"kind" binding:"required" enums:"Tool" example:"Tool"`
	Metadata   meta.ObjectMeta `json:"metadata" binding:"required"`
	Spec       ToolSpecJson    `json:"spec" binding:"required"`
	Status     *ToolStatusJson `json:"status,omitempty"`
}

// ToolSpecPatchJson documents a partial spec update. Omitted fields keep their
// current values, supplied fields replace the whole field, and null clears the
// optional output schema, hints, limits, and executors.
type ToolSpecPatchJson struct {
	ConnectionRef *meta.ObjectReference       `json:"connectionRef,omitempty"`
	Description   *string                     `json:"description,omitempty"`
	Verbs         []string                    `json:"verbs,omitempty"`
	InputSchema   map[string]interface{}      `json:"inputSchema,omitempty" swaggertype:"object"`
	OutputSchema  map[string]interface{}      `json:"outputSchema,omitempty" swaggertype:"object"`
	Hints         *toolschema.BehavioralHints `json:"hints,omitempty"`
	Limits        *toolschema.ExecutionLimits `json:"limits,omitempty"`
	ProxyHTTP     map[string]interface{}      `json:"proxyHttp,omitempty" swaggertype:"object"`
	Javascript    *string                     `json:"javascript,omitempty"`
}

// ToolPatchJson documents the PATCH /tools/{id} request body.
type ToolPatchJson struct {
	APIVersion string                `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string                `json:"kind" binding:"required" enums:"Tool" example:"Tool"`
	Metadata   *meta.ObjectMetaPatch `json:"metadata" binding:"required"`
	Spec       *ToolSpecPatchJson    `json:"spec" binding:"required"`
}

// ListToolsResponseJson documents the paginated management Tool list.
//
//	@Description	Paginated list of Tools
type ListToolsResponseJson struct {
	ResourceListJson
	Items []ToolJson `json:"items" binding:"required"`
}
