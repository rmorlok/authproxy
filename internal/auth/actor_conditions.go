package auth

import "github.com/rmorlok/authproxy/internal/database"

// ActorValidator is a function that validates an actor. It returns true if the actor is valid, false otherwise. If
// an actor is not valid for a request, a forbidden response should be returned.
type ActorValidator func(actor *database.Actor) (valid bool, reason string)

// ActorValidatorIsAdmin asserts that the actor is an admin.
func ActorValidatorIsAdmin(actor *database.Actor) (bool, string) {
	if actor.Admin {
		return true, ""
	}

	return false, "actor is not an admin"
}

// validateAllActorValidators validates all actor validators against the actor. It returns true if all validators
// pass, false otherwise.
func validateAllActorValidators(validators []ActorValidator, actor *database.Actor) (valid bool, reason string) {
	if actor == nil {
		return false, "no actor present"
	}

	for _, v := range validators {
		valid, reason = v(actor)
		if !valid {
			return false, reason
		}
	}

	return true, ""
}

// combineActorValidators combines multiple slices of actor validators into a single slice. It does not mutate the
// input slices.
func combineActorValidators(validators ...[]ActorValidator) []ActorValidator {
	result := make([]ActorValidator, 0, len(validators))

	for _, v := range validators {
		result = append(result, v...)
	}

	return result
}
