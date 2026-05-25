package connectors

import "github.com/rmorlok/authproxy/internal/schema/common"

// AuthValidator is implemented by Auth concrete types that have schema invariants
// to enforce. Connector.Validate type-asserts on this interface so an auth type
// without invariants (e.g. NoAuth) doesn't need to declare an empty Validate.
type AuthValidator interface {
	Validate(vc *common.ValidationContext) error
}
