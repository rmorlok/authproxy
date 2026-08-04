package config

import (
	"context"
	"strconv"
	"time"
)

type ServiceWorker struct {
	ServiceCommon             `json:",inline" yaml:",inline"`
	ConcurrencyVal            *StringValue   `json:"concurrency" yaml:"concurrency"`
	CronSyncInterval          *HumanDuration `json:"cronSyncInterval,omitempty" yaml:"cronSyncInterval,omitempty"`
	WorkflowPollers           *StringValue   `json:"workflowPollers,omitempty" yaml:"workflowPollers,omitempty"`
	ActivityPollers           *StringValue   `json:"activityPollers,omitempty" yaml:"activityPollers,omitempty"`
	MaxParallelWorkflowTasks  *StringValue   `json:"maxParallelWorkflowTasks,omitempty" yaml:"maxParallelWorkflowTasks,omitempty"`
	MaxParallelActivityTasks  *StringValue   `json:"maxParallelActivityTasks,omitempty" yaml:"maxParallelActivityTasks,omitempty"`
	WorkflowHeartbeatInterval *HumanDuration `json:"workflowHeartbeatInterval,omitempty" yaml:"workflowHeartbeatInterval,omitempty"`
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
