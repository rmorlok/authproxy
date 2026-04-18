package main

import (
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/spf13/cobra"
)

func main() {
	// Optionally load environment variables from .env files walking up
	// from the current working directory.
	util.LoadDotEnv()

	var rootCmd = &cobra.Command{
		Use: "ap",
	}

	rootCmd.AddCommand(cmdList())
	rootCmd.AddCommand(cmdSignJwt())
	rootCmd.AddCommand(cmdVerifyJwt())
	rootCmd.AddCommand(cmdRawProxy())
	rootCmd.AddCommand(cmdSignMarketplaceLoginUrl())
	rootCmd.AddCommand(cmdMarketplaceLoginRedirect())

	rootCmd.Execute()
}
