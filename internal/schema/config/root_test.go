package config

import (
	"testing"

	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
)

func TestRootRejectsLegacySnakeCaseKeys(t *testing.T) {
	var root Root
	require.Error(t, util.DecodeYAMLStrict([]byte("system_auth: {}\n"), &root))
}

func TestRootRejectsConnectorConfiguration(t *testing.T) {
	var root Root
	require.Error(t, util.DecodeYAMLStrict([]byte("connectors: {loadFromList: []}\n"), &root))
}
