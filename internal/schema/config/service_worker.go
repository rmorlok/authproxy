package config

import (
	"context"
	"strconv"
	"time"
)

type ServiceWorker struct {
	ServiceCommon             `json:",inline" yaml:",inline"`
	ConcurrencyVal            *StringValue   `json:"concurrency" yaml:"concurrency"`
	CronSyncInterval          *HumanDuration `json:"cron_sync_interval,omitempty" yaml:"cron_sync_interval,omitempty"`
	WorkflowPollers           *StringValue   `json:"workflow_pollers,omitempty" yaml:"workflow_pollers,omitempty"`
	ActivityPollers           *StringValue   `json:"activity_pollers,omitempty" yaml:"activity_pollers,omitempty"`
	MaxParallelWorkflowTasks  *StringValue   `json:"max_parallel_workflow_tasks,omitempty" yaml:"max_parallel_workflow_tasks,omitempty"`
	MaxParallelActivityTasks  *StringValue   `json:"max_parallel_activity_tasks,omitempty" yaml:"max_parallel_activity_tasks,omitempty"`
	WorkflowHeartbeatInterval *HumanDuration `json:"workflow_heartbeat_interval,omitempty" yaml:"workflow_heartbeat_interval,omitempty"`
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
	val := s.getOptionalInt(ctx, s.ConcurrencyVal)
	if val == nil {
		return 0
	}

	return *val
}

func (s *ServiceWorker) GetWorkflowPollers(ctx context.Context) *int {
	return s.getOptionalInt(ctx, s.WorkflowPollers)
}

func (s *ServiceWorker) GetActivityPollers(ctx context.Context) *int {
	return s.getOptionalInt(ctx, s.ActivityPollers)
}

func (s *ServiceWorker) GetMaxParallelWorkflowTasks(ctx context.Context) *int {
	return s.getOptionalInt(ctx, s.MaxParallelWorkflowTasks)
}

func (s *ServiceWorker) GetMaxParallelActivityTasks(ctx context.Context) *int {
	return s.getOptionalInt(ctx, s.MaxParallelActivityTasks)
}

func (s *ServiceWorker) GetWorkflowHeartbeatInterval() *time.Duration {
	if s == nil || s.WorkflowHeartbeatInterval == nil {
		return nil
	}

	return &s.WorkflowHeartbeatInterval.Duration
}

func (s *ServiceWorker) GetCronSyncInterval() time.Duration {
	if s == nil || s.CronSyncInterval == nil {
		return 5 * time.Minute
	}

	return s.CronSyncInterval.Duration
}

func (s *ServiceWorker) getOptionalInt(ctx context.Context, value *StringValue) *int {
	if s == nil || value == nil {
		return nil
	}

	rawValue, err := value.GetValue(ctx)
	if err != nil {
		return nil
	}

	parsedVal, err := strconv.Atoi(rawValue)
	if err != nil {
		return nil
	}

	return &parsedVal
}

var _ Service = (*ServiceWorker)(nil)
