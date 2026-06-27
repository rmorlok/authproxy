package oauth2

import (
	"context"
	"fmt"

	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

// effectiveScopes returns the computed, effective scopes for the the connection. It filters scopes
// that are removed via predicates, and translates the required value to a concrete value rather than
// a potentially requiring javascript evaluation.
func (o *oAuth2Connection) effectiveScopes(ctx context.Context) ([]sconfig.Scope, error) {
	if o.auth == nil || len(o.auth.Scopes) == 0 {
		return nil, nil
	}

	jsctx, err := o.connection.GetJavascriptContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("get javascript context for effective scopes: %w", err)
	}

	scopes := make([]sconfig.Scope, 0, len(o.auth.Scopes))
	for _, declared := range o.auth.Scopes {
		scope := declared.Clone()
		if scope.If != nil {
			ok, err := scope.If.GetValue(jsctx)
			if err != nil {
				return nil, fmt.Errorf("scope %q if.javascript: %w", scope.Id, err)
			}
			if !ok {
				continue
			}
		}

		if scope.Required != nil && scope.Required.Predicate != nil {
			required, err := scope.IsRequired(jsctx)
			if err != nil {
				return nil, fmt.Errorf("scope %q required.javascript: %w", scope.Id, err)
			}
			scope.Required = sconfig.NewScopeRequiredBool(required)
		}

		scopes = append(scopes, scope)
	}
	return scopes, nil
}
