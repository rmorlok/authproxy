package no_auth

import (
	"context"
	"testing"

	"github.com/rmorlok/authproxy/internal/auth_methods"
	"github.com/stretchr/testify/require"
)

func TestNoAuthAuthenticatorRefreshNoop(t *testing.T) {
	auth := NewAuthenticator()

	require.NoError(t, auth.Refresh(context.Background()))
	require.ErrorIs(t, auth.RecoverFrom401(context.Background()), auth_methods.ErrCannotRecover)
}
