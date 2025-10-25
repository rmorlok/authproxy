package dev_config

import (
	"testing"

	"github.com/rmorlok/authproxy/config"
	"github.com/stretchr/testify/require"
)

func Test_DevConfigValid(t *testing.T) {
	// This loads the dev config which also validates the schema to make sure the dev config
	// doesn't deviate from the defined schema as changes are added
	cfg, err := config.LoadConfig("./default.yaml")
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	err = cfg.Validate()
	require.NoError(t, err)
}
