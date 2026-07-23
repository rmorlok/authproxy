package main

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestRootCommandIncludesOnlyLoadTestCommands(t *testing.T) {
	rootCmd := newRootCommand()

	assert.Equal(t, "authproxy-loadtest", rootCmd.Name())
	assert.NotNil(t, rootCmd.Flag("config"))
	assert.NotNil(t, findCommand(rootCmd, "seed"))
	assert.NotNil(t, findCommand(rootCmd, "background"))
	assert.Nil(t, findCommand(rootCmd, "serve"))
}

func findCommand(rootCmd *cobra.Command, name string) *cobra.Command {
	for _, command := range rootCmd.Commands() {
		if command.Name() == name {
			return command
		}
	}
	return nil
}
