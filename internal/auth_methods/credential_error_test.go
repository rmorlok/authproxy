package auth_methods_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/rmorlok/authproxy/internal/auth_methods"
	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/stretchr/testify/require"
)

// TestMarkCredentialsUnavailable preserves the proxy's existing error contract
// while giving internal callers a classification independent of error text.
func TestMarkCredentialsUnavailable(t *testing.T) {
	// Plain failures and HTTP-aware failures both retain their original cause.
	cause := errors.New("private credential diagnostics")
	for _, original := range []error{
		fmt.Errorf("failed to load credential: %w", cause),
		httperr.New(http.StatusUnprocessableEntity, "no valid oauth token found", httperr.WithInternalErr(cause)),
	} {
		marked := auth_methods.MarkCredentialsUnavailable(original)
		require.EqualError(t, marked, original.Error())
		require.ErrorIs(t, marked, cause)
		require.ErrorIs(t, marked, original)
		require.ErrorIs(t, fmt.Errorf("additional context: %w", marked), auth_methods.ErrCredentialsUnavailable)
		require.Same(t, marked, auth_methods.MarkCredentialsUnavailable(marked))

		// Public proxy status/body mapping is identical, and errors.As still
		// returns the original HTTP-aware error when one was present.
		before, after := httperr.FromError(original), httperr.FromError(marked)
		require.Equal(t, before.Status, after.Status)
		require.Equal(t, before.ToErrorResponse(context.Background()), after.ToErrorResponse(context.Background()))
		require.Equal(t, httperr.As(original), httperr.As(marked))
	}

	// No category is inferred from a generic HTTP status or matching text.
	require.Nil(t, auth_methods.MarkCredentialsUnavailable(nil))
	require.NotErrorIs(t, httperr.New(http.StatusUnauthorized, "credentials unavailable"), auth_methods.ErrCredentialsUnavailable)
	require.NotErrorIs(t, errors.New(auth_methods.ErrCredentialsUnavailable.Error()), auth_methods.ErrCredentialsUnavailable)
}
