package migration

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIncompatibleErrorUnavailable(t *testing.T) {
	cause := errors.New("connection refused")

	// A status that could not be read becomes a typed UnavailableError that
	// keeps the historical message and unwraps to the cause.
	err := IncompatibleError(UnavailableStatus(TargetAppMetrics, "", 3, cause))
	var unavailable *UnavailableError
	require.ErrorAs(t, err, &unavailable)
	require.Equal(t, TargetAppMetrics, unavailable.Target)
	require.ErrorIs(t, err, cause)
	require.EqualError(t, err, "app-metrics schema is unavailable: connection refused")
}

func TestOnlyUnavailable(t *testing.T) {
	unavailableA := &UnavailableError{Target: TargetMainDatabase, Err: errors.New("down")}
	unavailableB := &UnavailableError{Target: TargetAppMetrics, Err: errors.New("memory limit")}
	stale := errors.New("app-metrics schema is behind")

	// nil and plain errors are not retryable.
	require.False(t, OnlyUnavailable(nil))
	require.False(t, OnlyUnavailable(stale))

	// Unavailable errors are retryable alone, wrapped, or joined together.
	require.True(t, OnlyUnavailable(unavailableA))
	require.True(t, OnlyUnavailable(fmt.Errorf("verify: %w", errors.Join(unavailableA, unavailableB))))

	// Any non-unavailable error in the tree makes the whole error non-retryable.
	require.False(t, OnlyUnavailable(fmt.Errorf("verify: %w", errors.Join(unavailableA, stale))))
}
