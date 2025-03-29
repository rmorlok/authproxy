package main

import (
	"github.com/fatih/color"
	"github.com/joho/godotenv"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/api_common"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/service/admin_api"
	api "github.com/rmorlok/authproxy/service/api"
	public "github.com/rmorlok/authproxy/service/public"
	"github.com/rmorlok/authproxy/service/worker"
	"github.com/spf13/cobra"
	"os"
	"strings"
	"sync"
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
	return errors.Wrapf(err, "failed to load configuration from '%s'", cfgFile)
}

func runServices(noBanner bool, servicesList string) error {
	services := strings.Split(servicesList, ",")
	servers := make([]func(cfg config.C), 0, len(services))

	if len(services) == 0 {
		return errors.New("no services provided")
	}
	for _, service := range services {
		switch service {
		case "admin-api":
			servers = append(servers, admin_api.Serve)
		case "api":
			servers = append(servers, api.Serve)
		case "public":
			servers = append(servers, public.Serve)
		case "worker":
			servers = append(servers, worker.Serve)
		case "all":
			servers = append(servers, admin_api.Serve, api.Serve, public.Serve, worker.Serve)
		default:
			return errors.New("unknown service: " + service)
		}
	}

	if !noBanner {
		banner()
	}

	wg := new(sync.WaitGroup)
	for _, server := range servers {
		wg.Add(1)
		go func(server func(cfg config.C)) {
			defer wg.Done()
			server(cfg)
		}(server)
	}

	wg.Wait()

	return nil
}

func banner() {
	banner := `
    ___         __  __       ____                       
   /   | __  __/ /_/ /_     / __ \_________  _  ____  __
  / /| |/ / / / __/ __ \   / /_/ / ___/ __ \| |/_/ / / /
 / ___ / /_/ / /_/ / / /  / ____/ /  / /_/ />  </ /_/ / 
/_/  |_\__,_/\__/_/ /_/  /_/   /_/   \____/_/|_|\__, /  
                                               /____/   
`
	color.Green(banner)
}

func cmdRoutes() *cobra.Command {
	return &cobra.Command{
		Use:   "routes",
		Short: "Print routes exposed by app",
		Run: func(cmd *cobra.Command, args []string) {
			api_common.PrintRoutes(admin_api.GetGinServer(cfg, nil, nil))
		},
	}
}

func cmdServe() *cobra.Command {
	var noBanner bool

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start services",
		Args:  cobra.ExactArgs(1), // Expect exactly one argument
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServices(noBanner, args[0])
		},
	}

	cmd.Flags().BoolVar(&noBanner, "no-banner", false, "Don't show banner")

	return cmd
}

func main() {
	// Optionally load environment variables from a .env file.
	_ = godotenv.Load()

	var rootCmd = &cobra.Command{
		Use: "authproxy",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return loadConfig()
		},
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file; may also be specified in AUTHPROXY_CONFIG")

	rootCmd.AddCommand(cmdRoutes())
	rootCmd.AddCommand(cmdServe())
	rootCmd.Execute()
}
