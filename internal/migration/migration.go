package migration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/mattn/go-sqlite3"
	"github.com/mitchellh/go-homedir"
	"github.com/rmorlok/authproxy/internal/schema/config"
)

type Target string

const (
	TargetMainDatabase Target = "main-database"
	TargetWorkflows    Target = "workflows"
	TargetAppMetrics   Target = "app-metrics"
	TargetAll          Target = "all"
)

var OrderedTargets = []Target{TargetMainDatabase, TargetWorkflows, TargetAppMetrics}

func ParseTarget(value string, allowAll bool) (Target, error) {
	target := Target(value)
	for _, valid := range OrderedTargets {
		if target == valid {
			return target, nil
		}
	}
	if allowAll && target == TargetAll {
		return target, nil
	}
	return "", fmt.Errorf("unknown migration database %q", value)
}

type Direction string

const (
	DirectionUp   Direction = "up"
	DirectionDown Direction = "down"
)

func ParseDirection(value string) (Direction, error) {
	if value == "" {
		return DirectionUp, nil
	}
	direction := Direction(value)
	if direction != DirectionUp && direction != DirectionDown {
		return "", fmt.Errorf("unknown migration direction %q; expected up or down", value)
	}
	return direction, nil
}

type State string

const (
	StateCurrent     State = "current"
	StateBehind      State = "behind"
	StateAhead       State = "ahead"
	StateDirty       State = "dirty"
	StateMissing     State = "missing"
	StateUnavailable State = "unavailable"
)

type Status struct {
	Target           Target
	Provider         config.DatabaseProvider
	CurrentVersion   *uint
	AvailableVersion uint
	Dirty            bool
	State            State
	Err              error
}

func (s Status) Compatible() bool {
	return s.State == StateCurrent && s.Err == nil
}

func (s Status) CurrentVersionString() string {
	if s.CurrentVersion == nil {
		return "none"
	}
	return strconv.FormatUint(uint64(*s.CurrentVersion), 10)
}

func NewStatus(target Target, provider config.DatabaseProvider, current *uint, available uint, dirty bool) Status {
	state := StateCurrent
	switch {
	case dirty:
		state = StateDirty
	case current == nil:
		state = StateMissing
	case *current < available:
		state = StateBehind
	case *current > available:
		state = StateAhead
	}
	return Status{
		Target:           target,
		Provider:         provider,
		CurrentVersion:   current,
		AvailableVersion: available,
		Dirty:            dirty,
		State:            state,
	}
}

func UnavailableStatus(target Target, provider config.DatabaseProvider, available uint, err error) Status {
	return Status{
		Target:           target,
		Provider:         provider,
		AvailableVersion: available,
		State:            StateUnavailable,
		Err:              err,
	}
}

var migrationFilename = regexp.MustCompile(`^(\d+).+\.up\.sql$`)

func LatestVersion(fsys fs.FS, directory string) (uint, error) {
	entries, err := fs.ReadDir(fsys, directory)
	if err != nil {
		return 0, err
	}
	var latest uint64
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		matches := migrationFilename.FindStringSubmatch(entry.Name())
		if len(matches) != 2 {
			continue
		}
		version, err := strconv.ParseUint(matches[1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse migration version from %q: %w", entry.Name(), err)
		}
		if version > latest {
			latest = version
		}
	}
	if latest == 0 {
		return 0, fmt.Errorf("no up migrations found in %q", directory)
	}
	return uint(latest), nil
}

func Inspect(ctx context.Context, target Target, cfg *config.Database, table string, available uint) Status {
	if cfg == nil || cfg.InnerVal == nil {
		return UnavailableStatus(target, "", available, errors.New("database configuration is required"))
	}
	if !validIdentifier(table) {
		return UnavailableStatus(target, cfg.GetProvider(), available, fmt.Errorf("invalid migration table %q", table))
	}

	current, dirty, err := inspect(ctx, cfg, table)
	if err != nil {
		return UnavailableStatus(target, cfg.GetProvider(), available, err)
	}
	return NewStatus(target, cfg.GetProvider(), current, available, dirty)
}

var identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func validIdentifier(value string) bool {
	return identifier.MatchString(value)
}

func inspect(ctx context.Context, cfg *config.Database, table string) (*uint, bool, error) {
	switch concrete := cfg.InnerVal.(type) {
	case *config.DatabaseSqlite:
		return inspectSQLite(ctx, concrete, table)
	case *config.DatabasePostgres:
		return inspectPostgres(ctx, concrete, table)
	case *config.DatabaseClickhouse:
		return inspectClickHouse(ctx, concrete, table)
	default:
		return nil, false, fmt.Errorf("unsupported database provider %q", cfg.GetProvider())
	}
}

func inspectSQLite(ctx context.Context, cfg *config.DatabaseSqlite, table string) (*uint, bool, error) {
	path, err := homedir.Expand(cfg.Path)
	if err != nil {
		return nil, false, fmt.Errorf("expand sqlite database path: %w", err)
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("stat sqlite database: %w", err)
	}

	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?mode=ro&_foreign_keys=on", path))
	if err != nil {
		return nil, false, fmt.Errorf("open sqlite database read-only: %w", err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return nil, false, fmt.Errorf("ping sqlite database read-only: %w", err)
	}

	var exists int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&exists); err != nil {
		return nil, false, fmt.Errorf("inspect sqlite migration table: %w", err)
	}
	if exists == 0 {
		return nil, false, nil
	}
	return readVersion(ctx, db, fmt.Sprintf(`SELECT version, dirty FROM "%s" LIMIT 1`, table))
}

func inspectPostgres(ctx context.Context, cfg *config.DatabasePostgres, table string) (*uint, bool, error) {
	db, err := sql.Open("pgx", cfg.GetDsn())
	if err != nil {
		return nil, false, fmt.Errorf("open postgres database read-only inspection: %w", err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return nil, false, fmt.Errorf("ping postgres database: %w", err)
	}

	var exists bool
	if err := db.QueryRowContext(ctx, `SELECT to_regclass($1) IS NOT NULL`, table).Scan(&exists); err != nil {
		return nil, false, fmt.Errorf("inspect postgres migration table: %w", err)
	}
	if !exists {
		return nil, false, nil
	}
	return readVersion(ctx, db, fmt.Sprintf(`SELECT version, dirty FROM "%s" LIMIT 1`, table))
}

func inspectClickHouse(ctx context.Context, cfg *config.DatabaseClickhouse, table string) (*uint, bool, error) {
	opts, err := cfg.ToClickhouseOptions()
	if err != nil {
		return nil, false, fmt.Errorf("resolve clickhouse configuration: %w", err)
	}
	db := sql.OpenDB(clickhouse.Connector(opts))
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return nil, false, fmt.Errorf("ping clickhouse database: %w", err)
	}

	databaseName := opts.Auth.Database
	var exists uint8
	if err := db.QueryRowContext(ctx, `SELECT count() > 0 FROM system.tables WHERE database = ? AND name = ?`, databaseName, table).Scan(&exists); err != nil {
		return nil, false, fmt.Errorf("inspect clickhouse migration table: %w", err)
	}
	if exists == 0 {
		return nil, false, nil
	}
	var version uint
	var dirty uint8
	err = db.QueryRowContext(ctx, fmt.Sprintf("SELECT version, dirty FROM `%s` ORDER BY sequence DESC LIMIT 1", table)).Scan(&version, &dirty)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read clickhouse migration version: %w", err)
	}
	return &version, dirty == 1, nil
}

func readVersion(ctx context.Context, db *sql.DB, query string) (*uint, bool, error) {
	var version uint
	var dirty bool
	err := db.QueryRowContext(ctx, query).Scan(&version, &dirty)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read migration version: %w", err)
	}
	return &version, dirty, nil
}

func Apply(ctx context.Context, migrator *migrate.Migrate, direction Direction, version *uint) error {
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			select {
			case migrator.GracefulStop <- true:
			case <-done:
			}
		case <-done:
		}
	}()

	var err error
	switch direction {
	case DirectionUp:
		if version == nil {
			err = migrator.Up()
		} else {
			err = migrator.Migrate(*version)
		}
	case DirectionDown:
		if version == nil {
			err = migrator.Steps(-1)
		} else {
			err = migrator.Migrate(*version)
		}
	default:
		return fmt.Errorf("unsupported migration direction %q", direction)
	}
	if errors.Is(err, migrate.ErrNoChange) {
		return nil
	}
	return err
}

func ValidateRequest(status Status, direction Direction, version *uint) error {
	if status.Err != nil {
		return status.Err
	}
	if status.Dirty {
		return fmt.Errorf("%s schema is dirty at version %s", status.Target, status.CurrentVersionString())
	}
	if version != nil && *version > status.AvailableVersion {
		return fmt.Errorf("%s requested version %d exceeds highest available version %d", status.Target, *version, status.AvailableVersion)
	}
	if status.CurrentVersion == nil {
		if direction == DirectionDown {
			return fmt.Errorf("%s schema is not initialized", status.Target)
		}
		return nil
	}
	if version == nil {
		if direction == DirectionUp && status.State == StateAhead {
			return fmt.Errorf("%s current version %d is ahead of highest available version %d", status.Target, *status.CurrentVersion, status.AvailableVersion)
		}
		return nil
	}
	if direction == DirectionUp && *version < *status.CurrentVersion {
		return fmt.Errorf("%s up target %d is below current version %d", status.Target, *version, *status.CurrentVersion)
	}
	if direction == DirectionDown && *version > *status.CurrentVersion {
		return fmt.Errorf("%s down target %d is above current version %d", status.Target, *version, *status.CurrentVersion)
	}
	return nil
}

func IncompatibleError(status Status) error {
	if status.Err != nil {
		return fmt.Errorf("%s schema is unavailable: %w", status.Target, status.Err)
	}
	if status.Compatible() {
		return nil
	}
	return fmt.Errorf(
		"%s schema is %s: current=%s expected=%d dirty=%t",
		status.Target,
		status.State,
		status.CurrentVersionString(),
		status.AvailableVersion,
		status.Dirty,
	)
}

func FormatTargets() string {
	values := make([]string, 0, len(OrderedTargets)+1)
	for _, target := range OrderedTargets {
		values = append(values, string(target))
	}
	values = append(values, string(TargetAll))
	return strings.Join(values, ", ")
}
