package config_test

import (
	"testing"

	apconfig "github.com/rmorlok/authproxy/internal/config"
	"github.com/stretchr/testify/require"
)

func TestIntegrationConfigValid(t *testing.T) {
	cfg, err := apconfig.LoadConfig("integration.yaml")
	require.NoError(t, err)
	require.NoError(t, cfg.Validate())
}
