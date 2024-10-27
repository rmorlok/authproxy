package main

import (
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

func main() {
	// Optionally load environment variables from a .env file.
	_ = godotenv.Load()

	var rootCmd = &cobra.Command{
		Use: "ap",
	}

	rootCmd.AddCommand(cmdList())
	rootCmd.AddCommand(cmdSignJwt())
	rootCmd.AddCommand(cmdVerifyJwt())

	rootCmd.Execute()
}
