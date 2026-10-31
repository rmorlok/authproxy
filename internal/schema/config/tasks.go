package config

import "time"

// DefaultWorkflowRetention is how long finished workflow instances are kept when
// tasks.workflowRetention is not configured.
const DefaultWorkflowRetention = 30 * 24 * time.Hour // 30 days

// Tasks configures how long the records of background work are kept.
type Tasks struct {
	// Default retention for tasks unless a value is explicitly set
	DefaultRetention *HumanDuration `json:"defaultRetention,omitempty" yaml:"defaultRetention,omitempty"`

	// WorkflowRetention is how long a finished workflow instance and its history are kept before the daily
	// cleanup task removes them. In-progress instances are never removed. If unset, defaults to 30 days. A
	// zero value disables the cleanup and keeps finished instances indefinitely.
	WorkflowRetention *HumanDuration `json:"workflowRetention,omitempty" yaml:"workflowRetention,omitempty"`
}

// GetWorkflowRetention returns how long finished workflow instances are kept, falling back to
// DefaultWorkflowRetention when unset. A result of zero or less means cleanup is disabled. Safe to call on a
// nil receiver, because the tasks block is optional.
func (t *Tasks) GetWorkflowRetention() time.Duration {
	if t == nil || t.WorkflowRetention == nil {
		return DefaultWorkflowRetention
	}

	return t.WorkflowRetention.Duration
}
