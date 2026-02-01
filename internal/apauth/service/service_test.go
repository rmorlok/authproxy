package service

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/internal/apauth/core"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/require"
)

func TestService_WithDefaultActorValidators(t *testing.T) {
	_, a1, _ := TestAuthService(t, sconfig.ServiceIdApi, nil)

	s1 := a1.(*service)
	require.Len(t, s1.defaultAuthValidators, 0)

	// Use a simple test validator instead of the removed AuthValidatorActorIsAdmin
	testValidator := func(gctx *gin.Context, ra *core.RequestAuth) (bool, string) {
		return true, ""
	}
	a2 := s1.WithDefaultAuthValidators(testValidator)

	s2 := a2.(*service)
	require.Len(t, s1.defaultAuthValidators, 0)
	require.Len(t, s2.defaultAuthValidators, 1)
}
