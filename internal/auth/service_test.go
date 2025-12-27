package auth

import (
	"testing"

	"github.com/rmorlok/authproxy/internal/config"
	"github.com/stretchr/testify/require"
)

func TestService_WithDefaultActorValidators(t *testing.T) {
	_, a1, _ := TestAuthService(t, config.ServiceIdApi, nil)

	s1 := a1.(*service)
	require.Len(t, s1.defaultAuthValidators, 0)

	a2 := s1.WithDefaultAuthValidators(AuthValidatorActorIsAdmin)

	s2 := a2.(*service)
	require.Len(t, s1.defaultAuthValidators, 0)
	require.Len(t, s2.defaultAuthValidators, 1)
}
