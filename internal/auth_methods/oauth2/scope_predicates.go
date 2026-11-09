package oauth2

import (
	"context"
	"fmt"

	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

// effectiveScopes returns copies of the connection's declared scopes, filtering
// excluded scopes and resolving dynamic requirements to booleans. Each predicate
// uses the connection's bounded evaluator with caller cancellation and fresh
// input copies. Static scopes do not load or initialize JavaScript.
func (o *oAuth2Connection) effectiveScopes(ctx context.Context) ([]sconfig.Scope, error) {
	if o.auth == nil || len(o.auth.Scopes) == 0 {
		return nil, nil
	}

	scopes := make([]sconfig.Scope, 0, len(o.auth.Scopes))
	for _, declared := range o.auth.Scopes {
		scope := declared.Clone()
		if scope.If != nil {
			ok, err := o.connection.EvaluateJavascriptPredicate(ctx, scope.If.Javascript)
			if err != nil {
				return nil, fmt.Errorf("scope %q if.javascript: %w", scope.Id, err)
			}
			if !ok {
				continue
			}
		}

		if scope.Required != nil && scope.Required.Predicate != nil {
			var required bool
			if scope.Required.Bool != nil {
				// Preserve ScopeRequired's boolean precedence for programmatically
				// constructed declarations containing both forms.
				required = *scope.Required.Bool
			} else {
				var err error
				required, err = o.connection.EvaluateJavascriptPredicate(ctx, scope.Required.Predicate.Javascript)
				if err != nil {
					return nil, fmt.Errorf("scope %q required.javascript: %w", scope.Id, err)
				}
			}
			scope.Required = sconfig.NewScopeRequiredBool(required)
		}

		scopes = append(scopes, scope)
	}
	return scopes, nil
}
