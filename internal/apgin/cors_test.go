package apgin

import (
	"testing"

	"github.com/gin-contrib/cors"
	"github.com/stretchr/testify/require"
)

// TestWithExposedHeaders verifies additions preserve policy and caller-owned slices.
func TestWithExposedHeaders(t *testing.T) {
	// Adding response metadata must not enable an otherwise disabled policy.
	require.Nil(t, WithExposedHeaders(nil, "X-Evidence"))

	// Preserve policy fields and configured exposure, including casing, without
	// appending into the caller's backing array or adding duplicate names.
	exposed := make([]string, 1, 4)
	exposed[0] = "x-existing"
	original := &cors.Config{
		AllowOrigins:     []string{"https://app.example.com"},
		AllowMethods:     []string{"POST"},
		AllowHeaders:     []string{"Authorization"},
		AllowCredentials: true,
		ExposeHeaders:    exposed,
	}
	updated := WithExposedHeaders(original, "X-Existing", "X-Evidence", "x-evidence")
	expected := *original
	expected.ExposeHeaders = []string{"x-existing", "X-Evidence"}
	require.Equal(t, &expected, updated)
	require.Equal(t, []string{"x-existing", "", "", ""}, exposed[:cap(exposed)])
}
