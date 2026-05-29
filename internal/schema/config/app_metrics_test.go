package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAppMetricsResourceSnapshotIntervalDefault(t *testing.T) {
	require.Equal(t, 15*time.Minute, (&AppMetrics{}).GetResourceSnapshotInterval())
	require.Equal(t, 15*time.Minute, (*AppMetrics)(nil).GetResourceSnapshotInterval())
}
