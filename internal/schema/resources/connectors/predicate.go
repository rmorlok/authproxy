package connectors

func connectorPredicateValidationVars() map[string]any {
	return map[string]any{
		"cfg":         map[string]any{},
		"labels":      map[string]string{},
		"annotations": map[string]string{},
	}
}
