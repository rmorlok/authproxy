package config

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	sq "github.com/Masterminds/squirrel"
	"github.com/go-faster/errors"
	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util"
)

// DatabaseClickhouse holds configuration for using ClickHouse as the HTTP logging database.
type DatabaseClickhouse struct {
	Provider                  DatabaseProvider `json:"provider" yaml:"provider"`
	Addresses                 []string         `json:"addresses,omitempty" yaml:"addresses,omitempty"`
	Address                   *StringValue     `json:"address,omitempty" yaml:"address,omitempty"`
	AddressList               *StringValue     `json:"address_list,omitempty" yaml:"address_list,omitempty"`
	Database                  *StringValue     `json:"database" yaml:"database"`
	User                      *StringValue     `json:"user,omitempty" yaml:"user,omitempty"`
	Password                  *StringValue     `json:"password,omitempty" yaml:"password,omitempty"`
	Protocol                  *string          `json:"protocol,omitempty" yaml:"protocol,omitempty"`
	AutoMigrate               bool             `json:"auto_migrate,omitempty" yaml:"auto_migrate,omitempty"`
	AutoMigrationLockDuration *HumanDuration   `json:"auto_migration_lock_duration,omitempty" yaml:"auto_migration_lock_duration,omitempty"`
}

func (d *DatabaseClickhouse) GetProvider() DatabaseProvider {
	return DatabaseProviderClickhouse
}

func (d *DatabaseClickhouse) GetDriver() string {
	return "clickhouse"
}

// GetProtocol returns the ClickHouse connection protocol. Defaults to HTTP if not set.
func (d *DatabaseClickhouse) GetProtocol() clickhouse.Protocol {
	if d.Protocol != nil && strings.ToLower(*d.Protocol) == "native" {
		return clickhouse.Native
	}
	return clickhouse.HTTP
}

func (d *DatabaseClickhouse) GetAutoMigrate() bool {
	return d.AutoMigrate
}

func (d *DatabaseClickhouse) GetAutoMigrationLockDuration() time.Duration {
	if d.AutoMigrationLockDuration == nil {
		return 2 * time.Minute
	}

	return d.AutoMigrationLockDuration.Duration
}

func (d *DatabaseClickhouse) GetAddresses(ctx context.Context) ([]string, error) {
	if len(d.Addresses) > 0 {
		return d.Addresses, nil
	}

	if d.AddressList != nil {
		list, err := d.AddressList.GetValue(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get clickhouse address list")
		}
		return strings.Split(list, ","), nil
	}

	if d.Address != nil {
		addr, err := d.Address.GetValue(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get clickhouse address")
		}
		return []string{addr}, nil
	}

	return nil, errors.New("no clickhouse addresses configured")
}

func (d *DatabaseClickhouse) GetUri() string {
	return d.buildUrl().String()
}

// GetDsn gets the Data Source Name
func (d *DatabaseClickhouse) GetDsn() string {
	return d.buildUrl().String()
}

func (d *DatabaseClickhouse) GetPlaceholderFormat() sq.PlaceholderFormat {
	return sq.Dollar
}

func (d *DatabaseClickhouse) Validate(vc *common.ValidationContext) error {
	result := &multierror.Error{}

	if d.Provider != DatabaseProviderClickhouse {
		return vc.NewErrorForField("provider", "provider must be 'clickhouse'")
	}

	address_fields := 0
	if len(d.Addresses) > 0 {
		address_fields++
	}
	if d.Address != nil {
		address_fields++
	}
	if d.AddressList != nil {
		address_fields++
	}

	if address_fields == 0 {
		return vc.NewErrorForField("address", "at least one address must be specified via addresses, address, or address_list")
	} else if address_fields > 1 {
		return vc.NewErrorForField("address", "only one of addresses, address, or address_list can be specified")
	}

	if d.Protocol != nil {
		p := strings.ToLower(*d.Protocol)
		if p != "http" && p != "native" {
			result = multierror.Append(result, vc.NewErrorForField("protocol", "protocol must be 'http' or 'native'"))
		}
	}

	if d.Database == nil {
		result = multierror.Append(result, vc.NewErrorForField("database", "database must be specified"))
	}

	return result.ErrorOrNil()
}

func (d *DatabaseClickhouse) buildUrl() *url.URL {
	ctx := context.Background()

	u := &url.URL{
		Scheme: "clickhouse",
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
	port := util.ToPtr(uint64(8123))
	addresses := util.Must(d.GetAddresses(ctx))
	if len(addresses) > 0 {
		addr := addresses[0]
		parts := strings.Split(addr, ":")
		host = parts[0]
		if len(parts) >= 2 {
			host = parts[0]
			port = util.ToPtr(util.Must(strconv.ParseUint(parts[1], 10, 64)))
		}
	}

	if port == nil {
		u.Host = host
	} else {
		u.Host = fmt.Sprintf("%s:%d", host, port)
	}

	return u
}

func (d *DatabaseClickhouse) ToClickhouseOptions() (*clickhouse.Options, error) {
	ctx := context.Background()
	addresses, err := d.GetAddresses(ctx)
	if err != nil {
		return nil, err
	}

	db, err := d.Database.GetValue(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get clickhouse database name")
	}

	cfg := &clickhouse.Options{
		Addr:     addresses,
		Protocol: d.GetProtocol(),
		Auth: clickhouse.Auth{
			Database: db,
		},
	}

	if d.User != nil {
		username, err := d.User.GetValue(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get clickhouse username")
		}

		cfg.Auth.Username = username
	}

	if d.Password != nil {
		password, err := d.Password.GetValue(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get clickhouse password")
		}

		cfg.Auth.Password = password
	}

	return cfg, nil
}
