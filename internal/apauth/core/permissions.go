package core

import (
	"slices"

	"github.com/rmorlok/authproxy/internal/aptmpl"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
)

// allowsForActor checks if this permission allows the specified action for the given actor.
//
// Parameters:
//   - namespace: The namespace where the action is being performed (e.g., "root.foo.bar").
//   - resource: The resource type being accessed (e.g., "connections", "connectors").
//   - verb: The action being performed (e.g., "get", "list", "create", "delete").
//   - resourceId: Optional. The specific resource ID being accessed. If empty, only checks
//     resource-level permission. If provided, the permission must either have no ResourceIds
//     (meaning all IDs are allowed) or include this specific ID.
//
// Matching rules:
//   - Namespace: Exact match, or if permission namespace ends with ".**", matches the base
//     namespace and all child namespaces.
//   - Resources: Exact match with any resource in the permission, or permission contains "*".
//   - Verbs: Exact match with any verb in the permission, or permission contains "*".
//   - ResourceIds: If permission has no ResourceIds, all IDs are allowed. If permission has
//     ResourceIds, the requested ID must be in the list.
func allowsForActor(actor *Actor, p aschema.Permission, namespace, resource, verb, resourceId string) bool {
	if !matchesNamespace(actor, p, namespace) {
		return false
	}

	if !matchesResource(p, resource) {
		return false
	}

	if !matchesVerb(p, verb) {
		return false
	}

	if !matchesResourceId(p, resourceId) {
		return false
	}

	return true
}

// matchesNamespace checks if this permission's namespace matches the target namespace.
// Supports wildcard matching with ".**" suffix.
func matchesNamespace(actor *Actor, p aschema.Permission, targetNamespace string) bool {
	if p.Namespace == "" || targetNamespace == "" {
		return false
	}

	if targetNamespace == namespace.NamespaceSkipNamespacePermissionChecks {
		return true
	}

	matcher, ok := renderValidPermissionNamespace(actor, p.Namespace)
	if !ok {
		return false
	}

	return namespace.NamespaceMatches(matcher, targetNamespace)
}

// renderPermissionNamespace applies templating to a given namespace string and returns
// the resulting string and whether the rendering was successful. If rendering was not
// successful, the namespace should be considered invalid.
func renderPermissionNamespace(actor *Actor, namespace string) (string, bool) {
	if !aptmpl.ContainsMustache(namespace) {
		return namespace, true
	}

	if actor == nil {
		return "", false
	}

	rendered, err := aptmpl.RenderMustache(namespace, actor.GetPermissionTemplateData())
	if err != nil {
		return "", false
	}

	return rendered, true
}

// renderValidPermissionNamespace optionally renders a namespace with templating based on actor data
// and validates that the resulting namespace is valid. If the templating cannot be fufulled, or the
// resulting namespace is not valid, it returns false.
func renderValidPermissionNamespace(actor *Actor, ns string) (string, bool) {
	rendered, ok := renderPermissionNamespace(actor, ns)
	if !ok {
		return "", false
	}

	if err := namespace.ValidateNamespaceMatcher(rendered); err != nil {
		return "", false
	}

	return rendered, true
}

// matchesResource checks if this permission allows access to the target resource.
// Supports wildcard matching with "*".
func matchesResource(p aschema.Permission, targetResource string) bool {
	if targetResource == "" {
		return false
	}

	for _, r := range p.Resources {
		if r == aschema.PermissionWildcard || r == targetResource {
			return true
		}
	}

	return false
}

// matchesVerb checks if this permission allows the target verb.
// Supports wildcard matching with "*".
func matchesVerb(p aschema.Permission, targetVerb string) bool {
	if targetVerb == "" {
		return false
	}

	for _, v := range p.Verbs {
		if v == aschema.PermissionWildcard || v == targetVerb {
			return true
		}
	}

	return false
}

// matchesResourceId checks if this permission allows access to the target resource ID.
// If the permission has no ResourceIds specified, all IDs are allowed.
// If the permission has ResourceIds, the target must be in the list.
func matchesResourceId(p aschema.Permission, targetResourceId string) bool {
	// If no specific resource IDs are required by the permission, allow all
	if len(p.ResourceIds) == 0 {
		return true
	}

	// If specific resource IDs are required but none provided in the request,
	// this is a resource-level check (e.g., list), which is allowed
	if targetResourceId == "" {
		return true
	}

	// Check if the target resource ID is in the allowed list
	return slices.Contains(p.ResourceIds, targetResourceId)
}

// permissionsAllowForActor checks if any permission in the slice allows the specified action for the given actor.
// Permissions are additive - if any single permission allows the action, it is permitted.
//
// This is the primary function for checking if an actor has permission to perform an action.
func permissionsAllowForActor(actor *Actor, permissions []aschema.Permission, namespace, resource, verb, resourceId string) bool {
	for _, p := range permissions {
		if allowsForActor(actor, p, namespace, resource, verb, resourceId) {
			return true
		}
	}

	return false
}

// permissionsAllowWithRestrictionsForActor checks if an action is allowed by both the actor's permissions
// and any additional request-level restrictions.
//
// This implements the intersection of two permission sets:
//   - The action must be allowed by at least one actor permission
//   - If restrictions are provided, the action must also be allowed by at least one restriction
//
// This is useful for scoped API tokens or temporary elevated/restricted permissions where
// a request should only be allowed if both the actor and the request context permit it.
//
// Parameters:
//   - actorPermissions: The permissions granted to the actor (user/service).
//   - restrictions: Optional additional restrictions. If nil or empty, only actor permissions are checked.
//   - namespace, resource, verb, resourceId: The action being checked.
func permissionsAllowWithRestrictionsForActor(
	actor *Actor,
	actorPermissions []aschema.Permission,
	restrictions []aschema.Permission,
	namespace, resource, verb, resourceId string,
) bool {
	// First check if the actor's permissions allow the action
	if !permissionsAllowForActor(actor, actorPermissions, namespace, resource, verb, resourceId) {
		return false
	}

	// If no restrictions are specified, the action is allowed
	if len(restrictions) == 0 {
		return true
	}

	// Check if the restrictions also allow the action
	return permissionsAllowForActor(actor, restrictions, namespace, resource, verb, resourceId)
}
