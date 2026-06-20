package apjs

// EvaluateBoolean runs a JavaScript expression with the supplied variables in
// scope and returns its boolean result. The expression must evaluate to a
// boolean; undefined, null, and other result types are rejected so callers can
// fail closed instead of guessing intent.
func EvaluateBoolean(expression string, vars map[string]any) (bool, error) {
	return NewContext(nil, vars).EvaluateBoolean(expression)
}
