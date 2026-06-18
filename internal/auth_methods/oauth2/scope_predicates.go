package oauth2

import (
	"context"
	"errors"
	"fmt"

	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

func (o *oAuth2Connection) effectiveScopes(ctx context.Context) ([]sconfig.Scope, error) {
	if o.auth == nil || len(o.auth.Scopes) == 0 {
		return nil, nil
	}

	var vars map[string]any
	getVars := func() (map[string]any, error) {
		if vars != nil {
			return vars, nil
		}
		if o.connection == nil {
			return nil, errors.New("connection is nil")
		}
		resolved, err := o.connection.GetPredicateVars(ctx)
		if err != nil {
			return nil, fmt.Errorf("get predicate vars: %w", err)
		}
		vars = resolved
		return vars, nil
	}

	scopes := make([]sconfig.Scope, 0, len(o.auth.Scopes))
	for _, declared := range o.auth.Scopes {
		scope := declared.Clone()
		if scope.If != nil {
			vars, err := getVars()
			if err != nil {
				return nil, err
			}
			ok, err := scope.If.GetValue(vars)
			if err != nil {
				return nil, fmt.Errorf("scope %q if.javascript: %w", scope.Id, err)
			}
			if !ok {
				continue
			}
		}

		if scope.Required != nil && scope.Required.Predicate != nil {
			vars, err := getVars()
			if err != nil {
				return nil, err
			}
			required, err := scope.IsRequired(vars)
			if err != nil {
				return nil, fmt.Errorf("scope %q required.javascript: %w", scope.Id, err)
			}
			scope.Required = sconfig.NewScopeRequiredBool(required)
		}

		scopes = append(scopes, scope)
	}
	return scopes, nil
}
