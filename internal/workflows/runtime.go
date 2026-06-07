package workflows

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"time"

	wfbackend "github.com/cschleiden/go-workflows/backend"
	"github.com/cschleiden/go-workflows/backend/history"
	"github.com/cschleiden/go-workflows/backend/postgres"
	"github.com/cschleiden/go-workflows/backend/sqlite"
	"github.com/cschleiden/go-workflows/client"
	wfcore "github.com/cschleiden/go-workflows/core"
	"github.com/cschleiden/go-workflows/diag"
	"github.com/cschleiden/go-workflows/registry"
	wfworker "github.com/cschleiden/go-workflows/worker"
	wflib "github.com/cschleiden/go-workflows/workflow"
	"github.com/golang-migrate/migrate/v4"
	migratedatabase "github.com/golang-migrate/migrate/v4/database"
	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	migratesqlite "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/rmorlok/authproxy/internal/aptelemetry"
	apdatabase "github.com/rmorlok/authproxy/internal/database"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

const (
	DefaultQueue = wflib.QueueDefault

	workflowMigrationsTable = "authproxy_workflows_schema_migrations"
)

//go:embed migrations/postgres/*.sql migrations/sqlite/*.sql
var migrationsFS embed.FS

type Runtime struct {
	backend wfbackend.Backend
	client  *client.Client
	db      *sql.DB
}

type Client interface {
	CreateWorkflowInstance(ctx context.Context, options client.WorkflowInstanceOptions, workflow wflib.Workflow, args ...any) (*wflib.Instance, error)
	GetWorkflowInstanceState(ctx context.Context, instance *wflib.Instance) (wfcore.WorkflowInstanceState, error)
	GetWorkflowInstanceHistory(ctx context.Context, instance *wflib.Instance, lastSequenceID *int64) ([]*history.Event, error)
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
		db, err = apdatabase.OpenInstrumentedSQL(
			"pgx",
			cfg.GetDsn(),
			apdatabase.DBSystemPostgreSQL,
			apdatabase.WithTelemetry(telemetry, root.Telemetry),
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

	switch cfg := root.Database.InnerVal.(type) {
	case *sconfig.DatabaseSqlite:
		db, err := sql.Open("sqlite", fmt.Sprintf("file:%v?_txlock=immediate", cfg.Path))
		if err != nil {
			return fmt.Errorf("open workflow sqlite database: %w", err)
		}
		defer db.Close()
		return migrateDB(db, "sqlite", "migrations/sqlite")
	case *sconfig.DatabasePostgres:
		db, err := apdatabase.OpenInstrumentedSQL("pgx", cfg.GetDsn(), apdatabase.DBSystemPostgreSQL)
		if err != nil {
			return fmt.Errorf("open workflow postgres database: %w", err)
		}
		defer db.Close()

		return migrateDB(db, "postgres", "migrations/postgres")
	default:
		return fmt.Errorf("workflow database provider %q is not supported", root.Database.GetProvider())
	}
}

func migrateDB(db *sql.DB, driverName string, sourcePath string) error {
	var (
		driver migratedatabase.Driver
		err    error
	)
	switch driverName {
	case "postgres":
		driver, err = migratepostgres.WithInstance(db, &migratepostgres.Config{
			MigrationsTable: workflowMigrationsTable,
		})
	case "sqlite":
		driver, err = migratesqlite.WithInstance(db, &migratesqlite.Config{
			MigrationsTable: workflowMigrationsTable,
		})
	default:
		return fmt.Errorf("workflow migration driver %q is not supported", driverName)
	}
	if err != nil {
		return fmt.Errorf("creating workflow migration instance: %w", err)
	}

	source, err := iofs.New(migrationsFS, sourcePath)
	if err != nil {
		return fmt.Errorf("creating workflow migration source: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", source, driverName, driver)
	if err != nil {
		return fmt.Errorf("creating workflow migration: %w", err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("running workflow migrations: %w", err)
	}

	return nil
}

func (r *Runtime) Backend() wfbackend.Backend {
	return r.backend
}

func (r *Runtime) DiagnosticBackend() (diag.Backend, error) {
	if r == nil || r.backend == nil {
		return nil, fmt.Errorf("workflow runtime is required")
	}

	backend, ok := r.backend.(diag.Backend)
	if !ok {
		return nil, fmt.Errorf("workflow backend does not support diagnostics")
	}

	return backend, nil
}

func (r *Runtime) Client() Client {
	return r
}

func (r *Runtime) CreateWorkflowInstance(ctx context.Context, options client.WorkflowInstanceOptions, workflow wflib.Workflow, args ...any) (*wflib.Instance, error) {
	return r.client.CreateWorkflowInstance(ctx, options, workflow, args...)
}

func (r *Runtime) GetWorkflowInstanceState(ctx context.Context, instance *wflib.Instance) (wfcore.WorkflowInstanceState, error) {
	return r.client.GetWorkflowInstanceState(ctx, instance)
}

func (r *Runtime) GetWorkflowInstanceHistory(ctx context.Context, instance *wflib.Instance, lastSequenceID *int64) ([]*history.Event, error) {
	return r.backend.GetWorkflowInstanceHistory(ctx, instance, lastSequenceID)
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
