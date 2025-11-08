package config

import "time"

type DatabaseSqlite struct {
	Provider                  DatabaseProvider `json:"provider" yaml:"provider"`
	Path                      string           `json:"path" yaml:"path"`
	AutoMigrate               bool             `json:"auto_migrate,omitempty" yaml:"auto_migrate,omitempty"`
	AutoMigrationLockDuration *HumanDuration   `json:"auto_migration_lock_duration,omitempty" yaml:"auto_migration_lock_duration,omitempty"`
}

func (d *DatabaseSqlite) GetProvider() DatabaseProvider {
	return DatabaseProviderSqlite
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
