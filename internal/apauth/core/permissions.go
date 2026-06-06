package core

import (
	"regexp"
	"slices"
	"strings"

	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
)

var permissionNamespaceTemplateRegex = regexp.MustCompile(`{{\s*([^{}]+?)\s*}}`)

// Allows checks if this permission allows the specified action.
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
func allows(p aschema.Permission, namespace, resource, verb, resourceId string) bool {
	return allowsForActor(nil, p, namespace, resource, verb, resourceId)
}

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

	if targetNamespace == aschema.NamespaceSkipNamespacePermissionChecks {
		return true
	}

	matcher, ok := renderValidPermissionNamespace(actor, p.Namespace)
	if !ok {
		return false
	}

	return aschema.NamespaceMatches(matcher, targetNamespace)
}

func renderPermissionNamespace(actor *Actor, namespace string) (string, bool) {
	if !strings.Contains(namespace, "{{") {
		return namespace, true
	}

	if actor == nil {
		return "", false
	}

	matches := permissionNamespaceTemplateRegex.FindAllStringSubmatchIndex(namespace, -1)
	if len(matches) == 0 {
		return "", false
	}

	var rendered strings.Builder
	last := 0
	for _, match := range matches {
		rendered.WriteString(namespace[last:match[0]])

		name := strings.TrimSpace(namespace[match[2]:match[3]])
		value, ok := actorPermissionTemplateValue(actor, name)
		if !ok {
			return "", false
		}

		rendered.WriteString(value)
		last = match[1]
	}
	rendered.WriteString(namespace[last:])

	result := rendered.String()
	if strings.Contains(result, "{{") || strings.Contains(result, "}}") {
		return "", false
	}

	return result, true
}

func renderValidPermissionNamespace(actor *Actor, namespace string) (string, bool) {
	rendered, ok := renderPermissionNamespace(actor, namespace)
	if !ok {
		return "", false
	}

	if err := aschema.ValidateNamespaceMatcher(rendered); err != nil {
		return "", false
	}

	return rendered, true
}

func actorPermissionTemplateValue(actor *Actor, name string) (string, bool) {
	switch {
	case name == "external_id":
		return actor.ExternalId, true
	case strings.HasPrefix(name, "labels."):
		key := strings.TrimPrefix(name, "labels.")
		if key == "" || actor.Labels == nil {
			return "", false
		}
		value, ok := actor.Labels[key]
		return value, ok
	case strings.HasPrefix(name, "annotations."):
		key := strings.TrimPrefix(name, "annotations.")
		if key == "" || actor.Annotations == nil {
			return "", false
		}
		value, ok := actor.Annotations[key]
		return value, ok
	default:
		return "", false
	}
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

// permissionsAllow checks if any permission in the slice allows the specified action.
// Permissions are additive - if any single permission allows the action, it is permitted.
//
// This is the primary function for checking if an actor has permission to perform an action.
func permissionsAllow(permissions []aschema.Permission, namespace, resource, verb, resourceId string) bool {
	return permissionsAllowForActor(nil, permissions, namespace, resource, verb, resourceId)
}

func permissionsAllowForActor(actor *Actor, permissions []aschema.Permission, namespace, resource, verb, resourceId string) bool {
	for _, p := range permissions {
		if allowsForActor(actor, p, namespace, resource, verb, resourceId) {
			return true
		}
	}

	return false
}

// permissionsAllowWithRestrictions checks if an action is allowed by both the actor's permissions
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
func permissionsAllowWithRestrictions(
	actorPermissions []aschema.Permission,
	restrictions []aschema.Permission,
	namespace, resource, verb, resourceId string,
) bool {
	return permissionsAllowWithRestrictionsForActor(nil, actorPermissions, restrictions, namespace, resource, verb, resourceId)
}

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
