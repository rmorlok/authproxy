package oauth2

import (
	"context"
	"errors"
	"fmt"

	"github.com/rmorlok/authproxy/internal/apjs"
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
		resolved, err := o.scopePredicateVars(ctx)
		if err != nil {
			return nil, err
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
			ok, err := apjs.EvaluateBoolean(scope.If.Javascript, vars)
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
			required, err := apjs.EvaluateBoolean(scope.Required.Predicate.Javascript, vars)
			if err != nil {
				return nil, fmt.Errorf("scope %q required.javascript: %w", scope.Id, err)
			}
			scope.Required = sconfig.NewScopeRequiredBool(required)
		}

		scopes = append(scopes, scope)
	}
	return scopes, nil
}

func (o *oAuth2Connection) scopePredicateVars(ctx context.Context) (map[string]any, error) {
	if o.connection == nil {
		return nil, errors.New("connection is nil")
	}

	cfg, err := o.connection.GetConfiguration(ctx)
	if err != nil {
		return nil, fmt.Errorf("get connection configuration: %w", err)
	}
	if cfg == nil {
		cfg = map[string]any{}
	}

	labels := o.connection.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}

	annotations := o.connection.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}

	return map[string]any{
		"cfg":         cfg,
		"labels":      labels,
		"annotations": annotations,
	}, nil
}
