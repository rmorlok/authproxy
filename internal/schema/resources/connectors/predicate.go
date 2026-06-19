package connectors

// connectorPredicateValidationVars returns the variables that are needed to run the javascript that is
// used in predicates for connector related values (setup steps, oauth scopes, etc). This just returns
// empty variables for all the required variable names.
func connectorPredicateValidationVars() map[string]any {
	return map[string]any{
		"cfg":         map[string]any{},
		"labels":      map[string]string{},
		"annotations": map[string]string{},
	}
}
