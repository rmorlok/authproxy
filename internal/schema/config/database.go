package config

import (
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/schema/common"
)

type DatabaseProvider string

const (
	DatabaseProviderSqlite     DatabaseProvider = "sqlite"
	DatabaseProviderPostgres   DatabaseProvider = "postgres"
	DatabaseProviderClickhouse DatabaseProvider = "clickhouse"
)

// DatabaseImpl is the interface implemented by concrete database configurations.
type DatabaseImpl interface {
	GetProvider() DatabaseProvider
	GetAutoMigrate() bool
	GetAutoMigrationLockDuration() time.Duration
	GetSoftDeleteRetention() *time.Duration
	GetUri() string
	GetDsn() string
	GetDriver() string
	GetPlaceholderFormat() sq.PlaceholderFormat
	Validate(vc *common.ValidationContext) error
}

// Database is the holder for a DatabaseImpl instance.
type Database struct {
	InnerVal DatabaseImpl `json:"-" yaml:"-"`
}

func (d *Database) GetProvider() DatabaseProvider {
	if d == nil || d.InnerVal == nil {
		return ""
	}
	return d.InnerVal.GetProvider()
}

func (d *Database) GetAutoMigrate() bool {
	if d == nil || d.InnerVal == nil {
		return false
	}
	return d.InnerVal.GetAutoMigrate()
}

func (d *Database) GetAutoMigrationLockDuration() time.Duration {
	if d == nil || d.InnerVal == nil {
		return 2 * time.Minute
	}
	return d.InnerVal.GetAutoMigrationLockDuration()
}

const DefaultSoftDeleteRetention = 30 * 24 * time.Hour // 30 days

func (d *Database) GetSoftDeleteRetention() *time.Duration {
	if d == nil || d.InnerVal == nil {
		return nil
	}
	return d.InnerVal.GetSoftDeleteRetention()
}

// GetSoftDeleteRetentionOrDefault returns the configured soft delete retention duration,
// or 30 days if not configured.
func (d *Database) GetSoftDeleteRetentionOrDefault() time.Duration {
	r := d.GetSoftDeleteRetention()
	if r == nil {
		return DefaultSoftDeleteRetention
	}
	return *r
}

func (d *Database) GetUri() string {
	if d == nil || d.InnerVal == nil {
		return ""
	}
	return d.InnerVal.GetUri()
}

func (d *Database) GetDsn() string {
	if d == nil || d.InnerVal == nil {
		return ""
	}
	return d.InnerVal.GetDsn()
}

func (d *Database) GetDriver() string {
	if d == nil || d.InnerVal == nil {
		return ""
	}
	return d.InnerVal.GetDriver()
}

func (d *Database) GetPlaceholderFormat() sq.PlaceholderFormat {
	if d == nil || d.InnerVal == nil {
		return sq.Question
	}
	return d.InnerVal.GetPlaceholderFormat()
}

func (d *Database) Validate(vc *common.ValidationContext) error {
	if d == nil || d.InnerVal == nil {
		return vc.NewError("database must be specified")
	}

	return d.InnerVal.Validate(vc)
}

var _ DatabaseImpl = (*Database)(nil)
