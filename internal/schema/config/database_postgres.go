package config

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
)

type DatabasePostgres struct {
	Provider                  DatabaseProvider  `json:"provider" yaml:"provider"`
	Host                      string            `json:"host" yaml:"host"`
	Port                      int               `json:"port,omitempty" yaml:"port,omitempty"`
	User                      string            `json:"user" yaml:"user"`
	Password                  string            `json:"password,omitempty" yaml:"password,omitempty"`
	Database                  string            `json:"database" yaml:"database"`
	SSLMode                   string            `json:"sslmode,omitempty" yaml:"sslmode,omitempty"`
	Params                    map[string]string `json:"params,omitempty" yaml:"params,omitempty"`
	AutoMigrate               bool              `json:"auto_migrate,omitempty" yaml:"auto_migrate,omitempty"`
	AutoMigrationLockDuration *HumanDuration    `json:"auto_migration_lock_duration,omitempty" yaml:"auto_migration_lock_duration,omitempty"`
}

func (d *DatabasePostgres) GetProvider() DatabaseProvider {
	return DatabaseProviderPostgres
}

func (d *DatabasePostgres) GetAutoMigrate() bool {
	return d.AutoMigrate
}

func (d *DatabasePostgres) GetAutoMigrationLockDuration() time.Duration {
	if d.AutoMigrationLockDuration == nil {
		return 2 * time.Minute
	}

	return d.AutoMigrationLockDuration.Duration
}

func (d *DatabasePostgres) GetUri() string {
	return d.buildUrl().String()
}

// GetDsn gets the Data Source Name
func (d *DatabasePostgres) GetDsn() string {
	return d.buildUrl().String()
}

func (d *DatabasePostgres) GetPlaceholderFormat() sq.PlaceholderFormat {
	return sq.Dollar
}

func (d *DatabasePostgres) buildUrl() *url.URL {
	u := &url.URL{
		Scheme: "postgres",
		Path:   d.Database,
	}

	if d.User != "" {
		if d.Password != "" {
			u.User = url.UserPassword(d.User, d.Password)
		} else {
			u.User = url.User(d.User)
		}
	}

	host := d.Host
	if host == "" {
		host = "localhost"
	}

	port := d.Port
	if port == 0 {
		port = 5432
	}

	u.Host = fmt.Sprintf("%s:%d", host, port)

	params := url.Values{}
	sslmode := d.SSLMode
	if sslmode == "" {
		sslmode = "disable"
	}
	params.Set("sslmode", sslmode)

	if len(d.Params) > 0 {
		keys := make([]string, 0, len(d.Params))
		for k := range d.Params {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if k == "" {
				continue
			}
			params.Set(k, d.Params[k])
		}
	}

	u.RawQuery = strings.TrimPrefix(params.Encode(), "&")
	return u
}
