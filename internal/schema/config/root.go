package config

import (
	"fmt"
	"log/slog"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
)

type Root struct {
	AdminApi        ServiceAdminApi `json:"admin_api" yaml:"admin_api"`
	Api             ServiceApi      `json:"api" yaml:"api"`
	Public          ServicePublic   `json:"public" yaml:"public"`
	Worker          ServiceWorker   `json:"worker" yaml:"worker"`
	Marketplace     *Marketplace    `json:"marketplace,omitempty" yaml:"marketplace,omitempty"`
	HostApplication HostApplication `json:"host_application" yaml:"host_application"`
	SystemAuth      SystemAuth      `json:"system_auth" yaml:"system_auth"`
	Database        *Database       `json:"database" yaml:"database"`
	Logging         *LoggingConfig  `json:"logging,omitempty" yaml:"logging,omitempty"`
	Redis           *Redis          `json:"redis" yaml:"redis"`
	Oauth           OAuth           `json:"oauth" yaml:"oauth"`
	ErrorPages      ErrorPages      `json:"error_pages,omitempty" yaml:"error_pages,omitempty"`
	Connectors      *Connectors     `json:"connectors" yaml:"connectors"`
	HttpLogging     *HttpLogging    `json:"http_logging,omitempty" yaml:"http_logging,omitempty"`
	Tasks           *Tasks          `json:"tasks,omitempty" yaml:"tasks,omitempty"`
	DevSettings     *DevSettings    `json:"dev_settings,omitempty" yaml:"dev_settings,omitempty"`
}

func (r *Root) GetRootLogger() *slog.Logger {
	if r == nil || r.Logging == nil {
		return (&LoggingConfigNone{Type: LoggingConfigTypeNone}).GetRootLogger()
	}

	return r.Logging.GetRootLogger()
}

func (r *Root) Validate() error {
	vc := &common.ValidationContext{Path: "$"}
	result := &multierror.Error{}

	if r.Connectors == nil {
		result = multierror.Append(result, vc.NewError("connectors block is required"))
	} else if err := r.Connectors.Validate(vc.PushField("connectors")); err != nil {
		result = multierror.Append(result, err)
	}

	if err := r.HostApplication.Validate(vc.PushField("host_application")); err != nil {
		result = multierror.Append(result, err)
	}

	if r.Database == nil {
		result = multierror.Append(result, vc.NewError("database block is required"))
	} else if err := r.Database.Validate(vc.PushField("database")); err != nil {
		result = multierror.Append(result, err)
	}

	return result.ErrorOrNil()
}

func (r *Root) MustGetService(serviceId ServiceId) Service {
	switch serviceId {
	case ServiceIdApi:
		return &r.Api
	case ServiceIdAdminApi:
		return &r.AdminApi
	case ServiceIdPublic:
		return &r.Public
	case ServiceIdWorker:
		return &r.Worker
	}

	panic(fmt.Sprintf("invalid service id %s", serviceId))
}

