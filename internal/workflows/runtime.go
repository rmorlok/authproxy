package workflows

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	wfbackend "github.com/cschleiden/go-workflows/backend"
	"github.com/cschleiden/go-workflows/backend/postgres"
	"github.com/cschleiden/go-workflows/backend/sqlite"
	"github.com/cschleiden/go-workflows/client"
	"github.com/cschleiden/go-workflows/registry"
	wfworker "github.com/cschleiden/go-workflows/worker"
	wflib "github.com/cschleiden/go-workflows/workflow"
	"github.com/rmorlok/authproxy/internal/aptelemetry"
	"github.com/rmorlok/authproxy/internal/database"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

const (
	DefaultQueue = wflib.QueueDefault
)

type Runtime struct {
	backend wfbackend.Backend
	client  *client.Client
	db      *sql.DB
}

func NewRuntime(root *sconfig.Root, telemetry *aptelemetry.Providers, logger *slog.Logger) (*Runtime, error) {
	if root == nil || root.Database == nil {
		return nil, fmt.Errorf("database configuration is required")
	}

	backendOptions := []wfbackend.BackendOption{
		wfbackend.WithLogger(logger),
	}
	if telemetry != nil && telemetry.TracerProvider != nil {
		backendOptions = append(backendOptions, wfbackend.WithTracerProvider(telemetry.TracerProvider))
	}

	var (
		b  wfbackend.Backend
		db *sql.DB
	)

	switch cfg := root.Database.InnerVal.(type) {
	case *sconfig.DatabaseSqlite:
		// Use the same database file as the primary application database, but
		// keep go-workflows migrations disabled here. AuthProxy runs migrations
		// centrally from DependencyManager so multi-worker startup is serialized.
		b = sqlite.NewSqliteBackend(
			cfg.Path,
			sqlite.WithApplyMigrations(false),
			sqlite.WithBackendOptions(backendOptions...),
		)
	case *sconfig.DatabasePostgres:
		var err error
		db, err = database.OpenInstrumentedSQL(
			"pgx",
			cfg.GetDsn(),
			database.DBSystemPostgreSQL,
			database.WithTelemetry(telemetry, root.Telemetry),
		)
		if err != nil {
			return nil, fmt.Errorf("open workflow postgres database: %w", err)
		}
		if err := db.Ping(); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("ping workflow postgres database: %w", err)
		}

		b = postgres.NewPostgresBackendWithDB(
			db,
			postgres.WithApplyMigrations(false),
			postgres.WithBackendOptions(backendOptions...),
		)
	default:
		return nil, fmt.Errorf("workflow database provider %q is not supported", root.Database.GetProvider())
	}

	return &Runtime{
		backend: b,
		client:  client.New(b),
		db:      db,
	}, nil
}

func Migrate(root *sconfig.Root, logger *slog.Logger) error {
	if root == nil || root.Database == nil {
		return fmt.Errorf("database configuration is required")
	}

	backendOptions := []wfbackend.BackendOption{
		wfbackend.WithLogger(logger),
	}

	switch cfg := root.Database.InnerVal.(type) {
	case *sconfig.DatabaseSqlite:
		b := sqlite.NewSqliteBackend(
			cfg.Path,
			sqlite.WithApplyMigrations(false),
			sqlite.WithBackendOptions(backendOptions...),
		)
		defer b.Close()
		return b.Migrate()
	case *sconfig.DatabasePostgres:
		db, err := database.OpenInstrumentedSQL("pgx", cfg.GetDsn(), database.DBSystemPostgreSQL)
		if err != nil {
			return fmt.Errorf("open workflow postgres database: %w", err)
		}
		defer db.Close()

		b := postgres.NewPostgresBackendWithDB(
			db,
			postgres.WithApplyMigrations(false),
			postgres.WithBackendOptions(backendOptions...),
		)
		return b.Migrate()
	default:
		return fmt.Errorf("workflow database provider %q is not supported", root.Database.GetProvider())
	}
}

func (r *Runtime) Backend() wfbackend.Backend {
	return r.backend
}

func (r *Runtime) Client() *client.Client {
	return r.client
}

func (r *Runtime) Ping(ctx context.Context) bool {
	if r == nil || r.backend == nil {
		return false
	}

	pingCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	_, err := r.backend.GetStats(pingCtx)
	return err == nil
}

func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}

	var closeErr error
	if r.backend != nil {
		closeErr = r.backend.Close()
	}
	if r.db != nil {
		if err := r.db.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	}

	return closeErr
}

type Worker struct {
	inner *wfworker.Worker
}

func NewWorker(runtime *Runtime, options *wfworker.Options) (*Worker, error) {
	if runtime == nil || runtime.backend == nil {
		return nil, fmt.Errorf("workflow runtime is required")
	}

	return &Worker{
		inner: wfworker.New(runtime.backend, options),
	}, nil
}

func (w *Worker) RegisterWorkflow(workflow wflib.Workflow, opts ...registry.RegisterOption) error {
	return w.inner.RegisterWorkflow(workflow, opts...)
}

func (w *Worker) RegisterActivity(activity wflib.Activity, opts ...registry.RegisterOption) error {
	return w.inner.RegisterActivity(activity, opts...)
}

func (w *Worker) Start(ctx context.Context) error {
	return w.inner.Start(ctx)
}

func (w *Worker) WaitForCompletion() error {
	return w.inner.WaitForCompletion()
}
