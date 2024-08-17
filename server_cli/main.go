package main

import (
	"github.com/joho/godotenv"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/admin_api"
	"github.com/rmorlok/authproxy/api_common"
	"github.com/rmorlok/authproxy/config"
	"github.com/spf13/cobra"
	"os"
	"strings"
	"sync"
)

var cfgFile string
var cfg config.Config

func loadConfig() error {
	if cfgFile == "" {
		cfgFile = os.Getenv("AUTHPROXY_CONFIG")
	}

	if cfgFile == "" {
		return errors.New("no configuration file found; must be specified with --config or AUTHPROXY_CONFIG environment variable")
	}

	var err error
	cfg, err = config.LoadConfig(cfgFile)
	return errors.Wrapf(err, "failed to load configuration from '%s'", cfgFile)
}

func runServices(servicesList string) error {
	services := strings.Split(servicesList, ",")
	servers := make([]func(cfg config.Config), 0, len(services))

	if len(services) == 0 {
		return errors.New("no services provided")
	}
	for _, service := range services {
		switch service {
		case "admin-api":
			servers = append(servers, admin_api.Serve)
		default:
			return errors.New("unknown service: " + service)
		}
	}

	wg := new(sync.WaitGroup)
	for _, server := range servers {
		wg.Add(1)
		go func(server func(cfg config.Config)) {
			defer wg.Done()
			server(cfg)
		}(server)
	}

	wg.Wait()

	return nil
}

func main() {
	// Optionally load environment variables from a .env file.
	_ = godotenv.Load()

	var cmdRoutes = &cobra.Command{
		Use:   "routes",
		Short: "Print routes exposed by app",
		Run: func(cmd *cobra.Command, args []string) {
			api_common.PrintRoutes(admin_api.GetGinServer(cfg))
		},
	}

	var cmdServe = &cobra.Command{
		Use:   "serve",
		Short: "Start services",
		Args:  cobra.ExactArgs(1), // Expect exactly one argument
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServices(args[0])
		},
	}

	var rootCmd = &cobra.Command{
		Use: "authproxy",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return loadConfig()
		},
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file; may also be specified in AUTHPROXY_CONFIG")

	rootCmd.AddCommand(cmdRoutes)
	rootCmd.AddCommand(cmdServe)
	rootCmd.Execute()
}
