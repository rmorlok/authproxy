package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"

	"github.com/fatih/color"
	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/internal/apgin"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/migration"
	"github.com/rmorlok/authproxy/internal/service"
	"github.com/rmorlok/authproxy/internal/service/admin_api"
	api "github.com/rmorlok/authproxy/internal/service/api"
	public "github.com/rmorlok/authproxy/internal/service/public"
	"github.com/rmorlok/authproxy/internal/service/worker"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/spf13/cobra"
)

var cfgFile string
var cfg config.C

type migrationManager interface {
	RunDevelopmentMigration(context.Context) error
	VerifyMigrations(context.Context) error
	RunProductionMigration(context.Context, migration.Target, migration.Direction, *uint) error
	MigrationStatuses(context.Context, migration.Target) []migration.Status
	ShutdownMigrationResources()
}

var newMigrationManager = func(serviceID string, cfg config.C) migrationManager {
	return service.NewDependencyManager(serviceID, cfg)
}

var startServices = runServices

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

	err = cfg.Validate()
	if err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	return nil
}

func runServices(noBanner bool, servicesList string) error {
	servers, err := resolveServices(servicesList)
	if err != nil {
		return err
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

func resolveServices(servicesList string) ([]func(cfg config.C), error) {
	services := strings.Split(servicesList, ",")
	servers := make([]func(cfg config.C), 0, len(services))

	if len(services) == 0 {
		return nil, errors.New("no services provided")
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
			return nil, errors.New("unknown service: " + service)
		}
	}

	return servers, nil
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
			println("Admin API:")
			server, _, _ := admin_api.GetGinServer(service.NewDependencyManager("admin-api", cfg))
			apgin.PrintRoutes(server.Handler.(*gin.Engine))

			println("\n\nAPI:")
			server, _, _ = api.GetGinServer(service.NewDependencyManager("api", cfg))
			apgin.PrintRoutes(server.Handler.(*gin.Engine))

			println("\n\nPublic:")
			server, _, _ = public.GetGinServer(service.NewDependencyManager("public", cfg))
			apgin.PrintRoutes(server.Handler.(*gin.Engine))
		},
	}
}

func cmdServe() *cobra.Command {
	var noBanner bool
	var autoMigrate bool

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start services",
		Args:  cobra.ExactArgs(1), // Expect exactly one argument
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := resolveServices(args[0]); err != nil {
				return err
			}
			if err := prepareServe(cmd.Context(), autoMigrate, cmd.ErrOrStderr()); err != nil {
				return err
			}
			return startServices(noBanner, args[0])
		},
	}

	cmd.Flags().BoolVar(&noBanner, "no-banner", false, "Don't show banner")
	cmd.Flags().BoolVar(&autoMigrate, "auto-migrate", false, "Automatically migrate and reconcile a local/disposable development environment (unsafe for production)")

	return cmd
}

func prepareServe(ctx context.Context, autoMigrate bool, warningOutput io.Writer) error {
	dm := newMigrationManager("startup-migrations", cfg)
	defer dm.ShutdownMigrationResources()
	if autoMigrate {
		fmt.Fprintln(warningOutput, "WARNING: --auto-migrate is intended only for local/disposable development environments. Use 'authproxy migrate' before production deployment.")
		if err := dm.RunDevelopmentMigration(ctx); err != nil {
			return fmt.Errorf("automatic development migration failed: %w", err)
		}
		return nil
	}
	return dm.VerifyMigrations(ctx)
}

func cmdMigrate() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate <database> [up|down] [version]",
		Short: "Migrate AuthProxy database schemas",
		Long:  "Migrate one AuthProxy schema target or all targets sequentially. Databases: " + migration.FormatTargets(),
		Args:  cobra.RangeArgs(1, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, direction, version, err := parseMigrateArgs(args)
			if err != nil {
				return err
			}
			dm := newMigrationManager("migrate", cfg)
			defer dm.ShutdownMigrationResources()
			return dm.RunProductionMigration(cmd.Context(), target, direction, version)
		},
	}
	cmd.AddCommand(cmdMigrateStatus())
	return cmd
}

func parseMigrateArgs(args []string) (migration.Target, migration.Direction, *uint, error) {
	target, err := migration.ParseTarget(args[0], true)
	if err != nil {
		return "", "", nil, fmt.Errorf("%w; expected one of: %s", err, migration.FormatTargets())
	}
	direction := migration.DirectionUp
	if len(args) >= 2 {
		direction, err = migration.ParseDirection(args[1])
		if err != nil {
			return "", "", nil, err
		}
	}
	var version *uint
	if len(args) == 3 {
		parsed, err := strconv.ParseUint(args[2], 10, 32)
		if err != nil {
			return "", "", nil, fmt.Errorf("invalid schema version %q: %w", args[2], err)
		}
		if parsed == 0 {
			return "", "", nil, errors.New("schema version must be greater than zero")
		}
		value := uint(parsed)
		version = &value
	}
	return target, direction, version, nil
}

func cmdMigrateStatus() *cobra.Command {
	return &cobra.Command{
		Use:   "status [database]",
		Short: "Report database schema migration status without making changes",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := migration.TargetAll
			if len(args) == 1 {
				var err error
				target, err = migration.ParseTarget(args[0], true)
				if err != nil {
					return fmt.Errorf("%w; expected one of: %s", err, migration.FormatTargets())
				}
			}
			dm := newMigrationManager("migrate-status", cfg)
			defer dm.ShutdownMigrationResources()
			statuses := dm.MigrationStatuses(cmd.Context(), target)
			printMigrationStatuses(cmd.OutOrStdout(), statuses)
			var result error
			for _, status := range statuses {
				result = errors.Join(result, migration.IncompatibleError(status))
			}
			return result
		},
	}
}

func printMigrationStatuses(output io.Writer, statuses []migration.Status) {
	w := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "DATABASE\tPROVIDER\tCURRENT\tAVAILABLE\tDIRTY\tSTATUS")
	for _, status := range statuses {
		fmt.Fprintf(
			w,
			"%s\t%s\t%s\t%d\t%t\t%s\n",
			status.Target,
			status.Provider,
			status.CurrentVersionString(),
			status.AvailableVersion,
			status.Dirty,
			status.State,
		)
	}
	_ = w.Flush()
}

func cmdReencrypt() *cobra.Command {
	return &cobra.Command{
		Use:   "reencrypt",
		Short: "Enqueue a background task to re-encrypt all data with the primary key",
		RunE: func(cmd *cobra.Command, args []string) error {
			dm := service.NewDependencyManager("reencrypt", cfg)
			task := encrypt.NewReencryptAllTask()
			info, err := dm.GetAsyncClient().Enqueue(task)
			if err != nil {
				return fmt.Errorf("failed to enqueue reencrypt task: %w", err)
			}
			fmt.Printf("Re-encryption task enqueued: id=%s queue=%s\n", info.ID, info.Queue)
			return nil
		},
	}
}

func newRootCommand() *cobra.Command {
	// Optionally load environment variables from .env files walking up
	// from the current working directory.
	util.LoadDotEnv()

	rootCmd := &cobra.Command{
		Use:          "authproxy",
		SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return loadConfig()
		},
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file; may also be specified in AUTHPROXY_CONFIG")

	rootCmd.AddCommand(cmdRoutes())
	rootCmd.AddCommand(cmdServe())
	rootCmd.AddCommand(cmdMigrate())
	rootCmd.AddCommand(cmdReencrypt())
	return rootCmd
}

func main() {
	if err := newRootCommand().Execute(); err != nil {
		os.Exit(1)
	}
}
