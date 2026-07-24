package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/spf13/cobra"
)

var cfgFile string
var cfg config.C

func loadConfig() error {
	if cfgFile == "" {
		cfgFile = os.Getenv("AUTHPROXY_CONFIG")
	}

	if cfgFile == "" {
		return errors.New("no configuration file found; must be specified with --config or AUTHPROXY_CONFIG environment variable")
	}

	var err error
	cfg, err = config.LoadConfig(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load configuration from '%s': %w", cfgFile, err)
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	return nil
}

func main() {
	// Load .env files for parity with the production server command.
	util.LoadDotEnv()

	rootCmd := &cobra.Command{
		Use: "authproxy-loadtest",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return loadConfig()
		},
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file; may also be specified in AUTHPROXY_CONFIG")
	rootCmd.AddCommand(cmdSeed())
	rootCmd.AddCommand(cmdBackground())
	rootCmd.Execute()
}
