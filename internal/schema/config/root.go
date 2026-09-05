package config

import (
	"fmt"
	"log/slog"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
)

type Root struct {
	AdminApi        ServiceAdminApi `json:"adminApi" yaml:"adminApi"`
	Api             ServiceApi      `json:"api" yaml:"api"`
	Public          ServicePublic   `json:"public" yaml:"public"`
	Worker          ServiceWorker   `json:"worker" yaml:"worker"`
	Marketplace     *Marketplace    `json:"marketplace,omitempty" yaml:"marketplace,omitempty"`
	HostApplication HostApplication `json:"hostApplication" yaml:"hostApplication"`
	SystemAuth      SystemAuth      `json:"systemAuth" yaml:"systemAuth"`
	Database        *Database       `json:"database" yaml:"database"`
	Logging         *LoggingConfig  `json:"logging,omitempty" yaml:"logging,omitempty"`
	Redis           *Redis          `json:"redis" yaml:"redis"`
	Oauth           OAuth           `json:"oauth" yaml:"oauth"`
	ErrorPages      ErrorPages      `json:"errorPages,omitempty" yaml:"errorPages,omitempty"`
	Connectors      *Connectors     `json:"connectors" yaml:"connectors"`
	RateLimits      *RateLimits     `json:"rateLimits,omitempty" yaml:"rateLimits,omitempty"`
	AppMetrics      *AppMetrics     `json:"appMetrics,omitempty" yaml:"appMetrics,omitempty"`
	Connections     *Connections    `json:"connections,omitempty" yaml:"connections,omitempty"`
	Tasks           *Tasks          `json:"tasks,omitempty" yaml:"tasks,omitempty"`
	Telemetry       *Telemetry      `json:"telemetry,omitempty" yaml:"telemetry,omitempty"`
	DevSettings     *DevSettings    `json:"devSettings,omitempty" yaml:"devSettings,omitempty"`
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

	if err := r.RateLimits.Validate(vc.PushField("rateLimits")); err != nil {
		result = multierror.Append(result, err)
	}

	if err := r.HostApplication.Validate(vc.PushField("host_application")); err != nil {
		result = multierror.Append(result, err)
	}

	if err := r.Telemetry.Validate(vc.PushField("telemetry")); err != nil {
		result = multierror.Append(result, err)
	}

	if r.Database == nil {
		result = multierror.Append(result, vc.NewError("database block is required"))
	} else if err := r.Database.Validate(vc.PushField("database")); err != nil {
		result = multierror.Append(result, err)
	}

	if err := r.AppMetrics.Validate(vc.PushField("app_metrics")); err != nil {
		result = multierror.Append(result, err)
	}

	if err := r.SystemAuth.DataEncryptionKeys.Validate(vc.PushField("system_auth").PushField("data_encryption_keys")); err != nil {
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
