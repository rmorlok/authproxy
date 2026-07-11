package api_key

import (
	"context"
	"testing"

	"github.com/rmorlok/authproxy/internal/auth_methods"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyAuthenticatorRefreshNoop(t *testing.T) {
	auth := &apiKeyConnection{}

	require.NoError(t, auth.Refresh(context.Background()))
	require.ErrorIs(t, auth.RecoverFrom401(context.Background()), auth_methods.ErrCannotRecover)
}
