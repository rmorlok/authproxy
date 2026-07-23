package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRootCommandExcludesLoadTestCommands(t *testing.T) {
	rootCmd := newRootCommand()
	commands := make(map[string]struct{})
	for _, command := range rootCmd.Commands() {
		commands[command.Name()] = struct{}{}
	}

	for _, commandName := range []string{"loadtest-seed", "loadtest-background", "seed", "background"} {
		assert.NotContains(t, commands, commandName)
	}
}
