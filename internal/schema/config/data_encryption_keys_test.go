package config

import (
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
)

func TestDataEncryptionKeysPolicy(t *testing.T) {
	now := time.Date(2026, time.June, 13, 12, 0, 0, 0, time.UTC)

	require.Equal(t, 90*24*time.Hour, (*DataEncryptionKeys)(nil).GetRotationInterval())
	require.True(t, (*DataEncryptionKeys)(nil).ShouldEnsureCurrent())
	require.False(t, (&DataEncryptionKeys{EnsureCurrent: util.ToPtr(false)}).ShouldEnsureCurrent())

	policy := &DataEncryptionKeys{RotationInterval: &HumanDuration{Duration: time.Hour}}
	require.False(t, policy.ShouldRotate(now, now.Add(-59*time.Minute)))
	require.True(t, policy.ShouldRotate(now, now.Add(-time.Hour)))

	disabled := &DataEncryptionKeys{RotationInterval: &HumanDuration{}}
	require.False(t, disabled.ShouldRotate(now, now.Add(-365*24*time.Hour)))

	invalid := &DataEncryptionKeys{RotationInterval: &HumanDuration{Duration: -time.Second}}
	require.Error(t, invalid.Validate(&common.ValidationContext{Path: "$"}))
}
