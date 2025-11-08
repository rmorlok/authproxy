package config

import (
	"context"
	"strconv"
	"time"
)

type ServiceWorker struct {
	ServiceCommon    `json:",inline" yaml:",inline"`
	ConcurrencyVal   *StringValue   `json:"concurrency" yaml:"concurrency"`
	CronSyncInterval *HumanDuration `json:"cron_sync_interval,omitempty" yaml:"cron_sync_interval,omitempty"`
}

func (s *ServiceWorker) HealthCheckPort() uint64 {
	p := s.ServiceCommon.healthCheckPort()
	if p != nil {
		return *p
	}

	return 0
}

func (s *ServiceWorker) GetId() ServiceId {
	return ServiceIdWorker
}

func (s *ServiceWorker) GetConcurrency(ctx context.Context) int {
	if s.ConcurrencyVal == nil {
		return 0
	}

	val, err := s.ConcurrencyVal.GetValue(ctx)
	if err != nil {
		return 0
	}

	parsedVal, err := strconv.Atoi(val)
	if err != nil {
		return 0
	}

	return parsedVal
}

func (s *ServiceWorker) GetCronSyncInterval() time.Duration {
	if s == nil || s.CronSyncInterval == nil {
		return 5 * time.Minute
	}

	return s.CronSyncInterval.Duration
}

var _ Service = (*ServicePublic)(nil)
