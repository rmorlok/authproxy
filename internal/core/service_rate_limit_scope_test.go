package core

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateRateLimitScopeTargetNamespace(t *testing.T) {
	for _, namespace := range []string{"root.acme", "root.acme.payments", "root.acme.payments.us"} {
		require.NoError(t, validateRateLimitScopeTargetNamespace("connector", "root.acme", namespace))
	}

	for _, namespace := range []string{"root", "root.other", "root.acmes", "root.ac"} {
		err := validateRateLimitScopeTargetNamespace("connection", "root.acme", namespace)
		require.ErrorIs(t, err, ErrInvalidArgument)
		require.ErrorContains(t, err, "outside rate-limit namespace")
	}
}
