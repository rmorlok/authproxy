package config

import (
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/schema/common"
)

type DatabaseProvider string

const (
	DatabaseProviderSqlite   DatabaseProvider = "sqlite"
	DatabaseProviderPostgres DatabaseProvider = "postgres"
)

// DatabaseImpl is the interface implemented by concrete database configurations.
type DatabaseImpl interface {
	GetProvider() DatabaseProvider
	GetAutoMigrate() bool
	GetAutoMigrationLockDuration() time.Duration
	GetUri() string
	GetDsn() string
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
