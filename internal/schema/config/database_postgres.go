package config

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util"
)

type DatabasePostgres struct {
	Provider                  DatabaseProvider  `json:"provider" yaml:"provider"`
	Host                      *StringValue      `json:"host" yaml:"host"`
	Port                      *IntegerValue     `json:"port,omitempty" yaml:"port,omitempty"`
	User                      *StringValue      `json:"user,omitempty" yaml:"user,omitempty"`
	Password                  *StringValue      `json:"password,omitempty" yaml:"password,omitempty"`
	Database                  *StringValue      `json:"database" yaml:"database"`
	SSLMode                   *StringValue      `json:"sslmode,omitempty" yaml:"sslmode,omitempty"`
	Params                    map[string]string `json:"params,omitempty" yaml:"params,omitempty"`
	MaxOpenConns              *IntegerValue     `json:"max_open_conns,omitempty" yaml:"max_open_conns,omitempty"`
	MaxIdleConns              *IntegerValue     `json:"max_idle_conns,omitempty" yaml:"max_idle_conns,omitempty"`
	ConnMaxLifetime           *HumanDuration    `json:"conn_max_lifetime,omitempty" yaml:"conn_max_lifetime,omitempty"`
	ConnMaxIdleTime           *HumanDuration    `json:"conn_max_idle_time,omitempty" yaml:"conn_max_idle_time,omitempty"`
	AutoMigrate               bool              `json:"auto_migrate,omitempty" yaml:"auto_migrate,omitempty"`
	AutoMigrationLockDuration *HumanDuration    `json:"auto_migration_lock_duration,omitempty" yaml:"auto_migration_lock_duration,omitempty"`
	SoftDeleteRetention       *HumanDuration    `json:"soft_delete_retention,omitempty" yaml:"soft_delete_retention,omitempty"`
}

func (d *DatabasePostgres) GetProvider() DatabaseProvider {
	return DatabaseProviderPostgres
}

func (d *DatabasePostgres) GetDriver() string {
	return "postgres"
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

func (d *DatabasePostgres) GetSoftDeleteRetention() *time.Duration {
	if d.SoftDeleteRetention == nil {
		return nil
	}
	return &d.SoftDeleteRetention.Duration
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
	ctx := context.Background()

	u := &url.URL{
		Scheme: "postgres",
	}

	if d.Database != nil {
		u.Path = util.Must(d.Database.GetValue(ctx))
	}

	user := ""
	if d.User != nil {
		user = util.Must(d.User.GetValue(ctx))
	}

	password := ""
	if d.Password != nil {
		password = util.Must(d.Password.GetValue(ctx))
	}

	if user != "" {
		if password != "" {
			u.User = url.UserPassword(user, password)
		} else {
			u.User = url.User(user)
		}
	}

	host := "localhost"
	if d.Host != nil {
		host = util.Must(d.Host.GetValue(ctx))
	}

	port := uint64(5432)
	if d.Port != nil {
		port = util.Must(d.Port.GetUint64Value(ctx))
	}

	u.Host = fmt.Sprintf("%s:%d", host, port)

	params := url.Values{}
	sslmode := "disable"
	if d.SSLMode != nil {
		sslmode = util.Must(d.SSLMode.GetValue(ctx))
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

func (d *DatabasePostgres) Validate(vc *common.ValidationContext) error {
	result := &multierror.Error{}

	if d.Host == nil {
		result = multierror.Append(result, vc.NewErrorForField("host", "host must be specified"))
	}

	if d.Database == nil {
		result = multierror.Append(result, vc.NewErrorForField("database", "database must be specified"))
	}

	if d.Port != nil {
		ctx := context.Background()
		port, err := d.Port.GetUint64Value(ctx)
		if err != nil {
			result = multierror.Append(result, vc.NewErrorfForField("port", "invalid port value: %v", err))
		} else if port == 0 || port > 65535 {
			result = multierror.Append(result, vc.NewErrorfForField("port", "port must be between 1 and 65535, got %d", port))
		}
	}
	result = multierror.Append(result, validateNonNegativeInteger(vc, "max_open_conns", d.MaxOpenConns))
	result = multierror.Append(result, validateNonNegativeInteger(vc, "max_idle_conns", d.MaxIdleConns))
	result = multierror.Append(result, validateNonNegativeDuration(vc, "conn_max_lifetime", d.ConnMaxLifetime))
	result = multierror.Append(result, validateNonNegativeDuration(vc, "conn_max_idle_time", d.ConnMaxIdleTime))

	return result.ErrorOrNil()
}

func validateNonNegativeInteger(vc *common.ValidationContext, field string, value *IntegerValue) error {
	if value == nil {
		return nil
	}
	v, err := value.GetValue(context.Background())
	if err != nil {
		return vc.NewErrorfForField(field, "invalid value: %v", err)
	}
	if v < 0 {
		return vc.NewErrorfForField(field, "must be greater than or equal to 0, got %d", v)
	}
	return nil
}

func validateNonNegativeDuration(vc *common.ValidationContext, field string, value *HumanDuration) error {
	if value == nil {
		return nil
	}
	if value.Duration < 0 {
		return vc.NewErrorfForField(field, "must be greater than or equal to 0, got %s", value.Duration)
	}
	return nil
}
