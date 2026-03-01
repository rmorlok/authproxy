package core

import (
	"context"
	"slices"

	"github.com/rmorlok/authproxy/internal/apid"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
)

const authContextKey = "auth"

// GetAuthFromContext gets the auth from context. If no auth is in context, it returns an unauthenticated auth.
func GetAuthFromContext(ctx context.Context) *RequestAuth {
	if a, ok := ctx.Value(authContextKey).(*RequestAuth); ok {
		return a
	}

	return NewUnauthenticatedRequestAuth()
}

// RequestAuth contains authentication and authorization information for a request.
//
// It includes the authenticated actor (user/service) and optionally request-level
// permission restrictions that further limit what the request can do beyond the
// actor's base permissions.
type RequestAuth struct {
	sessionId *apid.ID
	actor     *Actor

	// permissions restricts what actions this specific request can perform.
	// If nil or empty, the actor's full permissions apply.
	// If set, both the actor's permissions AND these restrictions must allow the action.
	// This enables scoped API tokens, temporary permission grants, etc.
	permissions []aschema.Permission
}

func (ra *RequestAuth) IsAuthenticated() bool {
	return ra.actor != nil
}

func (ra *RequestAuth) IsSession() bool {
	return ra.IsAuthenticated() && ra.sessionId != nil
}

func (ra *RequestAuth) GetSessionId() *apid.ID {
	return ra.sessionId
}

func (ra *RequestAuth) SetSessionId(sessionId *apid.ID) {
	ra.sessionId = sessionId
}

func (ra *RequestAuth) GetActor() *Actor {
	return ra.actor
}

func (ra *RequestAuth) MustGetActor() *Actor {
	if ra.actor == nil {
		panic("expected request to be authenticated")
	}

	return ra.actor
}

func (a *RequestAuth) ContextWith(ctx context.Context) context.Context {
	return context.WithValue(ctx, authContextKey, a)
}

// GetPermissions returns the request-level permission restrictions.
// Returns nil if no restrictions are set.
func (ra *RequestAuth) GetPermissions() []aschema.Permission {
	return ra.permissions
}

// SetPermissions sets request-level permission restrictions.
// When set, actions must be allowed by both actor permissions AND these restrictions.
func (ra *RequestAuth) SetPermissions(permissions []aschema.Permission) {
	ra.permissions = permissions
}

func (ra *RequestAuth) GetNamespacesAllowed(resource, verb string) []string {
	if ra == nil || !ra.IsAuthenticated() {
		return nil
	}

	candidateNamespaces := make([]string, 0, len(ra.actor.Permissions))
	for _, permission := range ra.actor.Permissions {
		appliesToResource := slices.Contains(permission.Resources, resource) ||
			slices.Contains(permission.Resources, aschema.PermissionWildcard)
		appliesToVerb := slices.Contains(permission.Verbs, verb) ||
			slices.Contains(permission.Verbs, aschema.PermissionWildcard)

		if appliesToResource && appliesToVerb {
			candidateNamespaces = append(candidateNamespaces, permission.Namespace)
		}
	}

	var finalNamespaces []string

	if len(ra.permissions) > 0 {
		finalNamespaces = make([]string, 0, len(candidateNamespaces))

		for _, permission := range ra.permissions {
			appliesToResource := slices.Contains(permission.Resources, resource) ||
				slices.Contains(permission.Resources, aschema.PermissionWildcard)
			appliesToVerb := slices.Contains(permission.Verbs, verb) ||
				slices.Contains(permission.Verbs, aschema.PermissionWildcard)

			if appliesToResource && appliesToVerb {
				for _, candidateNamespace := range candidateNamespaces {
					if restricted, ok := aschema.NamespaceMatcherConstrained(permission.Namespace, candidateNamespace); ok {
						finalNamespaces = append(finalNamespaces, restricted)
					}
				}
			}
		}
	} else {
		finalNamespaces = candidateNamespaces
	}

	// Only return unique namespaces
	slices.Sort(finalNamespaces)
	return slices.Compact(finalNamespaces)
}

// Allows checks if this request is authorized to perform the specified action.
//
// Authorization is granted if:
//  1. The actor is authenticated
//  2. The actor's permissions allow the action (or actor is admin/superadmin)
//  3. If request-level restrictions are set, they also allow the action
//
// Parameters:
//   - namespace: The namespace where the action is being performed
//   - resource: The resource type being accessed (e.g., "connections")
//   - verb: The action being performed (e.g., "get", "list", "create")
//   - resourceId: Optional specific resource ID being accessed
func (ra *RequestAuth) Allows(namespace, resource, verb, resourceId string) bool {
	if ra == nil || !ra.IsAuthenticated() {
		return false
	}

	actor := ra.GetActor()

	// Check actor permissions with optional request-level restrictions
	return permissionsAllowWithRestrictions(
		actor.GetPermissions(),
		ra.permissions,
		namespace, resource, verb, resourceId,
	)
}

// AllowsReason is like Allows but returns a reason string if the action is not allowed.
// This is useful for logging and debugging.
func (ra *RequestAuth) AllowsReason(namespace, resource, verb, resourceId string) (allowed bool, reason string) {
	if ra == nil {
		return false, "request auth is nil"
	}

	if !ra.IsAuthenticated() {
		return false, "actor not authenticated"
	}

	actor := ra.GetActor()

	// Check actor permissions
	if !permissionsAllow(actor.GetPermissions(), namespace, resource, verb, resourceId) {
		return false, "actor permissions do not allow this action"
	}

	// Check request-level restrictions if present
	if len(ra.permissions) > 0 {
		if !permissionsAllow(ra.permissions, namespace, resource, verb, resourceId) {
			return false, "request permissions do not allow this action"
		}
	}

	// This should be true, but fall back to allows as the source of truth.
	return ra.Allows(namespace, resource, verb, resourceId), ""
}

func NewUnauthenticatedRequestAuth() *RequestAuth {
	return &RequestAuth{}
}

func NewAuthenticatedRequestAuth(a IActorData) *RequestAuth {
	return &RequestAuth{
		actor: CreateActor(a),
	}
}

func NewAuthenticatedRequestAuthWithSession(a IActorData, sess *apid.ID) *RequestAuth {
	return &RequestAuth{
		actor:     CreateActor(a),
		sessionId: sess,
	}
}

// NewAuthenticatedRequestAuthWithPermissions creates a RequestAuth with both an actor
// and request-level permission restrictions.
func NewAuthenticatedRequestAuthWithPermissions(a IActorData, permissions []aschema.Permission) *RequestAuth {
	return &RequestAuth{
		actor:       CreateActor(a),
		permissions: permissions,
	}
}

// NewAuthenticatedRequestAuthWithSessionAndPermissions creates a RequestAuth with an actor,
// session, and request-level permission restrictions.
func NewAuthenticatedRequestAuthWithSessionAndPermissions(a IActorData, sess *apid.ID, permissions []aschema.Permission) *RequestAuth {
	return &RequestAuth{
		actor:       CreateActor(a),
		sessionId:   sess,
		permissions: permissions,
	}
}
