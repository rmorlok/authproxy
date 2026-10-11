package routes

import (
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
)

// idObjectReference builds an object reference that identifies a resource of
// the given kind solely by its immutable ID. ID-based GET routes use this to
// load resources through the core Resolve*Reference methods so that lookup
// validation, soft-delete handling, and hydration match reference resolution
// elsewhere in the system. For namespaces the ID is the canonical path.
func idObjectReference(kind meta.Kind, id string) meta.ObjectReference {
	return meta.ObjectReference{
		APIVersion: meta.APIVersionV1Alpha1,
		Kind:       kind,
		ID:         id,
	}
}
