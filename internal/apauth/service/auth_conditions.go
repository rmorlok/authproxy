package service

// AuthValidator is a function that validates the auth for a request. It returns true if the auth is valid,
// false otherwise. If an actor is not valid for a request, a forbidden response should be returned.
type AuthValidator func(ra *RequestAuth) (valid bool, reason string)

// AuthValidatorActorIsAdmin asserts that the actor is an admin.
func AuthValidatorActorIsAdmin(ra *RequestAuth) (bool, string) {
	if ra == nil {
		return false, "auth not present"
	}

	if !ra.IsAuthenticated() {
		return false, "actor not authenticated"
	}

	if ra.GetActor().Admin {
		return true, ""
	}

	return false, "actor is not an admin"
}

// validateAllActorValidators validates all actor validators against the actor. It returns true if all validators
// pass, false otherwise.
func validateAllAuthValidators(validators []AuthValidator, ra *RequestAuth) (valid bool, reason string) {
	if ra == nil {
		return false, "auth present"
	}

	for _, v := range validators {
		valid, reason = v(ra)
		if !valid {
			return false, reason
		}
	}

	return true, ""
}

// combineActorValidators combines multiple slices of actor validators into a single slice. It does not mutate the
// input slices.
func combineAuthValidators(validators ...[]AuthValidator) []AuthValidator {
	result := make([]AuthValidator, 0, len(validators))

	for _, v := range validators {
		result = append(result, v...)
	}

	return result
}
