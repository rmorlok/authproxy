package main

import (
	"context"
	"fmt"
	"time"

	encryptpkg "github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/loadtest/seeder"
	"github.com/rmorlok/authproxy/internal/service"
	"github.com/spf13/cobra"
)

func cmdLoadtestSeed() *cobra.Command {
	var profilePath string
	var runDir string
	var providerBaseURL string
	var migrate bool
	var distributedMigrationLocks bool
	var verifySamples int
	var progressEvery int
	var oauthExpiringPercent int
	var periodicProbePercent int

	cmd := &cobra.Command{
		Use:   "loadtest-seed",
		Short: "Seed AuthProxy resources for load-test profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			if profilePath == "" {
				return fmt.Errorf("--profile is required")
			}

			profile, err := seeder.LoadProfile(profilePath)
			if err != nil {
				return fmt.Errorf("failed to load profile: %w", err)
			}

			dm := service.NewDependencyManager("loadtest-seed", cfg)
			defer dm.ShutdownTelemetry()
			defer dm.ShutdownDatabase()

			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			if migrate {
				if distributedMigrationLocks {
					dm.AutoMigrateDatabase()
					dm.AutoMigrateGenerateDataEncryptionKeys()
					dm.AutoMigrateSyncKeysToDatabase()
				} else {
					if err := dm.GetDatabase().Migrate(ctx); err != nil {
						return fmt.Errorf("failed to migrate database: %w", err)
					}
					if err := encryptpkg.GenerateDataEncryptionKeysToDatabase(ctx, cfg, dm.GetDatabase(), dm.GetLogger(), nil); err != nil {
						return fmt.Errorf("failed to generate data encryption keys: %w", err)
					}
					if err := encryptpkg.SyncKeysToDatabase(ctx, cfg, dm.GetDatabase(), dm.GetLogger(), nil); err != nil {
						return fmt.Errorf("failed to sync keys to database: %w", err)
					}
				}
			}

			enc := dm.GetEncryptService()
			defer enc.Shutdown()
			if err := enc.SyncKeysFromDbToMemory(ctx); err != nil {
				return fmt.Errorf("failed to sync encryption keys: %w", err)
			}

			var oauthOverride *int
			if cmd.Flags().Changed("oauth-expiring-percent") {
				oauthOverride = &oauthExpiringPercent
			}
			var probeOverride *int
			if cmd.Flags().Changed("periodic-probe-percent") {
				probeOverride = &periodicProbePercent
			}

			result, err := seeder.Seed(ctx, seeder.Options{
				Profile:              profile,
				DB:                   dm.GetDatabase(),
				Encrypt:              enc,
				ProviderBaseURL:      providerBaseURL,
				OAuthExpiringPercent: oauthOverride,
				PeriodicProbePercent: probeOverride,
				VerifySamples:        verifySamples,
				ProgressEvery:        progressEvery,
				Now:                  time.Now().UTC(),
				Logf: func(format string, args ...any) {
					fmt.Printf("[loadtest-seed] "+format+"\n", args...)
				},
			})
			if err != nil {
				return err
			}

			if err := seeder.WriteArtifacts(runDir, result); err != nil {
				return fmt.Errorf("failed to write seed artifacts: %w", err)
			}

			fmt.Printf(
				"Seeded profile %s: namespaces=%d connections=%d oauth_tokens=%d verified=%d\n",
				result.ProfileName,
				result.CreatedNamespaces,
				len(result.Connections),
				result.UpsertedOAuthTokens,
				len(result.VerifiedSamples),
			)
			if runDir != "" {
				fmt.Printf("Run artifacts: %s\n", runDir)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&profilePath, "profile", "", "load-test profile YAML")
	cmd.Flags().StringVar(&runDir, "run-dir", "", "artifact directory to populate")
	cmd.Flags().StringVar(&providerBaseURL, "provider-base-url", "", "go-oauth2-server base URL used in seeded connector definitions")
	cmd.Flags().BoolVar(&migrate, "migrate", true, "run database and encryption-key migrations before seeding")
	cmd.Flags().BoolVar(&distributedMigrationLocks, "distributed-migration-locks", false, "use Redis-backed distributed migration locks")
	cmd.Flags().IntVar(&verifySamples, "verify-samples", 3, "number of seeded connections to verify through AuthProxy internals")
	cmd.Flags().IntVar(&progressEvery, "progress-every", 1000, "log progress every N connections")
	cmd.Flags().IntVar(&oauthExpiringPercent, "oauth-expiring-percent", 0, "override percent of tokens expiring inside the refresh window")
	cmd.Flags().IntVar(&periodicProbePercent, "periodic-probe-percent", 0, "override percent of connections marked for periodic probes")

	return cmd
}
