package config

import (
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/schema/common"
)

type DatabaseSqlite struct {
	Provider                  DatabaseProvider `json:"provider" yaml:"provider"`
	Path                      string           `json:"path" yaml:"path"`
	AutoMigrate               bool             `json:"auto_migrate,omitempty" yaml:"auto_migrate,omitempty"`
	AutoMigrationLockDuration *HumanDuration   `json:"auto_migration_lock_duration,omitempty" yaml:"auto_migration_lock_duration,omitempty"`
}

func (d *DatabaseSqlite) GetProvider() DatabaseProvider {
	return DatabaseProviderSqlite
}

func (d *DatabaseSqlite) GetDriver() string {
	return "sqlite3"
}

func (d *DatabaseSqlite) GetAutoMigrate() bool {
	return d.AutoMigrate
}

func (d *DatabaseSqlite) GetAutoMigrationLockDuration() time.Duration {
	if d.AutoMigrationLockDuration == nil {
		return 2 * time.Minute
	}

	return d.AutoMigrationLockDuration.Duration
}

func (d *DatabaseSqlite) GetUri() string {
	return fmt.Sprintf("sqlite3://%s?_foreign_keys=on&_journal_mode=WAL", d.Path)
}

// GetDsn gets the Data Source Name
func (d *DatabaseSqlite) GetDsn() string {
	return fmt.Sprintf("file:%s?_foreign_keys=on&_journal_mode=WAL", d.Path)
}

func (d *DatabaseSqlite) GetPlaceholderFormat() sq.PlaceholderFormat {
	return sq.Question
}

func (d *DatabaseSqlite) Validate(vc *common.ValidationContext) error {
	if d.Path == "" {
		return vc.NewErrorForField("path", "path must be specified")
	}

	return nil
}
