package iface_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/rmorlok/authproxy/internal/core/iface"
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
		marked := iface.MarkCredentialsUnavailable(original)
		require.EqualError(t, marked, original.Error())
		require.ErrorIs(t, marked, cause)
		require.ErrorIs(t, marked, original)
		require.ErrorIs(t, fmt.Errorf("additional context: %w", marked), iface.ErrCredentialsUnavailable)
		require.Same(t, marked, iface.MarkCredentialsUnavailable(marked))

		// Public proxy status/body mapping is identical, and errors.As still
		// returns the original HTTP-aware error when one was present.
		before, after := httperr.FromError(original), httperr.FromError(marked)
		require.Equal(t, before.Status, after.Status)
		require.Equal(t, before.ToErrorResponse(context.Background()), after.ToErrorResponse(context.Background()))
		require.Equal(t, httperr.As(original), httperr.As(marked))
	}

	// No category is inferred from a generic HTTP status or matching text.
	require.Nil(t, iface.MarkCredentialsUnavailable(nil))
	require.NotErrorIs(t, httperr.New(http.StatusUnauthorized, "credentials unavailable"), iface.ErrCredentialsUnavailable)
	require.NotErrorIs(t, errors.New(iface.ErrCredentialsUnavailable.Error()), iface.ErrCredentialsUnavailable)
}
