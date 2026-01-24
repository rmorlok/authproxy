package service

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/api_common"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
)

type hasId interface {
	GetId() uuid.UUID
}

type hasNamespace interface {
	GetNamespace() string
}

// IdExtractor is a function that can extract an id from an object to the value use in the resource ids field of
// permissions.
type IdExtractor func(interface{}) string

func DefaultIdExtractor(obj interface{}) string {
	hi, ok := obj.(hasId)

	if !ok {
		panic("could not extract id from object")
	}

	return hi.GetId().String()
}

const validatorContextKey = "validatorContextKey"

// MustGetValidatorFromContext gets a resource validator that can be used to validate or filter resources from
// context. If no validator is present, it panics.
func MustGetValidatorFromContext(ctx context.Context) *ResourcePermissionValidator {
	if a, ok := ctx.Value(validatorContextKey).(*ResourcePermissionValidator); ok {
		return a
	}

	panic("no resource validator present in context")
}

func FilterForValidatedResources[T any](validator *ResourcePermissionValidator, objs []T) []T {
	// Even if there are no objects to validate, validation has been done
	validator.MarkValidated()

	filtered := make([]T, 0, len(objs))
	for _, obj := range objs {
		if validator.Validate(obj) == nil {
			filtered = append(filtered, obj)
		}
	}

	return filtered
}

// ResourcePermissionValidator is an object that can validate a loaded resource conforms to what is allowed by
// the route's definition and the request auth's permissions. Routes need to validate resources after loaded because
// namespace cannot be validated at the HTTP layer.
type ResourcePermissionValidator struct {
	pvb              *PermissionValidatorBuilder
	ra               *core.RequestAuth
	hasBeenValidated bool
	errorReturn      bool
}

func (rpv *ResourcePermissionValidator) MarkValidated() {
	rpv.hasBeenValidated = true
}

func (rpv *ResourcePermissionValidator) MarkErrorReturn() {
	rpv.errorReturn = true
}

func (rpv *ResourcePermissionValidator) ContextWith(ctx context.Context) context.Context {
	return context.WithValue(ctx, validatorContextKey, rpv)
}

// ValidateNamespaceResourceId validates that the actor has permission to perform the route's action on the specified
// resource in the given namespace. A non-nil error implies the actor does not have access to the resource.
func (rpv *ResourcePermissionValidator) ValidateNamespaceResourceId(ns, resourceId string) error {
	rpv.hasBeenValidated = true

	allowed, reason := rpv.ra.AllowsReason(ns, rpv.pvb.resource, rpv.pvb.verb, resourceId)
	if !allowed {
		return fmt.Errorf("permission denied: %s", reason)
	}

	return nil
}

// ValidateNamespace validates that the actor has permission to perform the verb for the route in the specified
// namespace. This validation would be used when a specific resource is not present, such as when creating a new
// resource.
func (rpv *ResourcePermissionValidator) ValidateNamespace(ns string) error {
	return rpv.ValidateNamespaceResourceId(ns, "")
}

// Validate validates that the actor has permission to access the resource. This is used to validate existing objects
// in the system. The namespace and id are automatically extracted from the object using the extractor provided
// when configuring the route. A non-nil error implies the actor does not have access to the resource.
func (rpv *ResourcePermissionValidator) Validate(obj interface{}) error {
	getNsObj, ok := obj.(hasNamespace)
	if !ok {
		panic("object does not implement namespace retrieval")
	}

	ns := getNsObj.GetNamespace()

	idExtractor := DefaultIdExtractor
	if rpv.pvb.idExtractor != nil {
		idExtractor = rpv.pvb.idExtractor
	}

	resourceId := idExtractor(obj)

	return rpv.ValidateNamespaceResourceId(ns, resourceId)
}

// ValidateHttpStatusError does the same validation as Validate, but returns a HttpStatusError instead of an error.
func (rpv *ResourcePermissionValidator) ValidateHttpStatusError(obj interface{}) *api_common.HttpStatusError {
	err := rpv.Validate(obj)
	if err == nil {
		return nil
	}

	return api_common.
		NewHttpStatusErrorBuilder().
		WithStatusForbidden().
		WithPublicErr(err).
		BuildStatusError()
}

// GetEffectiveNamespaceMatchers computes the namespace matchers that should be applied to a database query.
// `queryMatcher` should be the namespace matcher received as a query param to an endpoint intented as filter.
//
// It returns:
// - the allowed namespaces for the resource/verb, optionally intersected with queryMatcher
// - a slice with NamespaceNoMatchSentinel if no namespaces can be matched
func (rpv *ResourcePermissionValidator) GetEffectiveNamespaceMatchers(queryMatcher *string) []string {
	if rpv.ra == nil || !rpv.ra.IsAuthenticated() {
		return []string{aschema.NamespaceNoMatchSentinel} // No access if not authenticated
	}

	// Get the namespaces this actor can access for this resource/verb
	allowedNamespaces := rpv.ra.GetNamespacesAllowed(rpv.pvb.resource, rpv.pvb.verb)
	if len(allowedNamespaces) == 0 {
		return []string{} // Empty slice means no access
	}

	// If no query filter, return the allowed namespaces directly
	if queryMatcher == nil {
		return allowedNamespaces
	}

	// Intersect each allowed namespace with the query matcher
	result := make([]string, 0, len(allowedNamespaces))
	for _, allowed := range allowedNamespaces {
		if constrained, ok := aschema.NamespaceMatcherConstrained(allowed, *queryMatcher); ok {
			result = append(result, constrained)
		}
	}

	if len(result) == 0 {
		// Return a set that indicates that no resources can be matched because the intersection of the allowed
		// namespaces and the requested namespaces is an empty set.
		return []string{aschema.NamespaceNoMatchSentinel}
	}

	return result
}

// PermissionValidatorBuilder constructs gin middleware that validates permissions for a request.
//
// The builder follows a fluent pattern where you chain method calls to configure the
// permission requirements:
//
//	auth.NewRequiredBuilder().
//		ForResource("connections").
//		ForVerb("get").
//		ForIdField("id").
//		Build()
//
// Permission Checking:
//   - The middleware first authenticates the request (same as Required())
//   - Then checks if the actor's permissions allow the specified resource/verb combination
//   - If an ID field is specified, also checks resourceId restrictions
//
// Namespace Handling:
//   - If no namespace is specified, defaults to "root"
//   - ForNamespace() sets a static namespace
//   - ForNamespaceQueryParam() extracts namespace from a query parameter
//   - ForNamespacePathParam() extracts namespace from a URL path parameter
type PermissionValidatorBuilder struct {
	s                   *service
	resource            string
	verb                string
	idField             string
	namespace           string
	namespaceQueryParam string
	namespacePathParam  string
	idExtractor         IdExtractor
}

// ForResource sets the resource type being accessed (e.g., "connections", "connectors").
// This is required.
func (pb *PermissionValidatorBuilder) ForResource(resource string) *PermissionValidatorBuilder {
	pb.resource = resource
	return pb
}

// ForVerb sets the action being performed (e.g., "get", "list", "create", "delete").
// This is required.
func (pb *PermissionValidatorBuilder) ForVerb(verb string) *PermissionValidatorBuilder {
	pb.verb = verb
	return pb
}

// ForIdField sets the URL path parameter name that contains the resource ID.
// When set, the validator will extract the ID from the path and check resourceId restrictions.
// Example: ForIdField("id") will extract the ID from "/connections/:id"
func (pb *PermissionValidatorBuilder) ForIdField(idField string) *PermissionValidatorBuilder {
	pb.idField = idField
	return pb
}

// ForIdExtractor sets a method that can be used to extract the id field on the objects that this endpoint deals with.
// This allows for post-validation of objects after they are loaded from the database.
func (pb *PermissionValidatorBuilder) ForIdExtractor(idExtractor IdExtractor) *PermissionValidatorBuilder {
	pb.idExtractor = idExtractor
	return pb
}

// ForNamespace sets a static namespace for permission checking.
// Use this when all requests to this route operate in a fixed namespace.
func (pb *PermissionValidatorBuilder) ForNamespace(namespace string) *PermissionValidatorBuilder {
	pb.namespace = namespace
	return pb
}

// ForNamespaceQueryParam specifies a query parameter to extract the namespace from.
// Example: ForNamespaceQueryParam("namespace") extracts from "?namespace=root.foo"
func (pb *PermissionValidatorBuilder) ForNamespaceQueryParam(param string) *PermissionValidatorBuilder {
	pb.namespaceQueryParam = param
	return pb
}

// ForNamespacePathParam specifies a URL path parameter to extract the namespace from.
// Example: ForNamespacePathParam("ns") extracts from "/namespaces/:ns/..."
func (pb *PermissionValidatorBuilder) ForNamespacePathParam(param string) *PermissionValidatorBuilder {
	pb.namespacePathParam = param
	return pb
}

// getNamespace extracts the namespace from the request based on configuration.
// Priority: path param > query param > static namespace > default "root"
func (pb *PermissionValidatorBuilder) getNamespace(c *gin.Context) string {
	// First try path parameter
	if pb.namespacePathParam != "" {
		if ns := c.Param(pb.namespacePathParam); ns != "" {
			return ns
		}
	}

	// Then try query parameter
	if pb.namespaceQueryParam != "" {
		if ns := c.Query(pb.namespaceQueryParam); ns != "" {
			return ns
		}
	}

	// Then use static namespace
	if pb.namespace != "" {
		return pb.namespace
	}

	// Default to skipping permission check at the namespace level
	return aschema.NamespaceSkipNamespacePermissionChecks
}

// getResourceId extracts the resource ID from the request if an ID field is configured.
func (pb *PermissionValidatorBuilder) getResourceId(c *gin.Context) string {
	if pb.idField == "" {
		return ""
	}

	return c.Param(pb.idField)
}

// Build creates a gin.HandlerFunc that:
// 1. Requires authentication (like Required())
// 2. Validates that the actor has permission for the configured resource/verb/namespace
//
// Permission checking is performed after authentication. Admins and SuperAdmins
// bypass all permission checks. Regular actors must have permissions that allow
// the requested action.
//
// Returns 401 Unauthorized if not authenticated.
// Returns 403 Forbidden if the actor lacks the required permission.
func (pb *PermissionValidatorBuilder) Build() gin.HandlerFunc {
	if pb.resource == "" {
		panic("resource must be specified")
	}

	if pb.verb == "" {
		panic("verb must be specified")
	}

	// Create a permission-checking AuthValidator
	permissionValidator := func(gctx *gin.Context, ra *core.RequestAuth) (valid bool, reason string) {
		if ra == nil || !ra.IsAuthenticated() {
			return false, "not authenticated"
		}

		validator := &ResourcePermissionValidator{
			pvb: pb,
			ra:  ra,
		}

		applyValidatorToGinContext(gctx, validator)

		return ra.AllowsReason(pb.getNamespace(gctx), pb.resource, pb.verb, pb.getResourceId(gctx))
	}

	postValidator := func(gctx *gin.Context, ra *core.RequestAuth) {
		v := MustGetValidatorFromGinContext(gctx)
		if !v.hasBeenValidated && !v.errorReturn {
			panic("resources not validated in endpoint call")
		}
	}

	return pb.s.requiredWithPostValidation([]AuthValidator{permissionValidator}, postValidator)
}
