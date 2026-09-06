package api

import (
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
)

// ErrorResponse is the standardized error response format for authproxy API errors.
//
//	@Description	Standardized error response
type ErrorResponse struct {
	// Error message.
	Error string `json:"error" yaml:"error" example:"Bad Request"`
	// Stack trace, only populated in debug mode.
	StackTrace string `json:"stackTrace,omitempty" yaml:"stackTrace,omitempty"`
}

// SessionInitiateParams is the request body for POST /session/_initiate.
// ReturnToUrl is where the browser should land after host authentication.
type SessionInitiateParams struct {
	ReturnToUrl string `json:"returnToUrl" yaml:"returnToUrl" example:"https://example.com/return"`
}

// SessionInitiateFailureResponse tells the SPA where to redirect when the
// current request cannot establish a session yet.
type SessionInitiateFailureResponse struct {
	RedirectUrl string `json:"redirectUrl" yaml:"redirectUrl" example:"https://example.com/auth"`
}

// SessionInitiateSuccessResponse is returned once a session already exists or
// has been established from the request authentication.
type SessionInitiateSuccessResponse struct {
	ActorId apid.ID `json:"actorId" yaml:"actorId" swaggertype:"string" example:"act_test550e8400abcde"`
}

type KeyValueJson struct {
	Key   string `json:"key" yaml:"key" example:"env"`
	Value string `json:"value" yaml:"value" example:"production"`
}

type PutKeyValueRequestJson struct {
	Value string `json:"value" yaml:"value" example:"production"`
}

type TaskState string

const (
	TaskStateUnknown   TaskState = "unknown"
	TaskStateActive    TaskState = "active"
	TaskStatePending   TaskState = "pending"
	TaskStateScheduled TaskState = "scheduled"
	TaskStateRetry     TaskState = "retry"
	TaskStateFailed    TaskState = "failed"
	TaskStateCompleted TaskState = "completed"
)

type TaskInfoJson struct {
	Id        string     `json:"id" yaml:"id"`
	Type      string     `json:"type" yaml:"type"`
	State     TaskState  `json:"state" yaml:"state"`
	UpdatedAt *time.Time `json:"updatedAt,omitempty" yaml:"updatedAt,omitempty"`
}

type QueueInfoJson struct {
	Queue          string  `json:"queue" yaml:"queue"`
	MemoryUsage    int64   `json:"memoryUsage" yaml:"memoryUsage"`
	Latency        float64 `json:"latencySeconds" yaml:"latencySeconds"`
	Size           int     `json:"size" yaml:"size"`
	Groups         int     `json:"groups" yaml:"groups"`
	Pending        int     `json:"pending" yaml:"pending"`
	Active         int     `json:"active" yaml:"active"`
	Scheduled      int     `json:"scheduled" yaml:"scheduled"`
	Retry          int     `json:"retry" yaml:"retry"`
	Archived       int     `json:"archived" yaml:"archived"`
	Completed      int     `json:"completed" yaml:"completed"`
	Aggregating    int     `json:"aggregating" yaml:"aggregating"`
	Processed      int     `json:"processed" yaml:"processed"`
	Failed         int     `json:"failed" yaml:"failed"`
	ProcessedTotal int     `json:"processedTotal" yaml:"processedTotal"`
	FailedTotal    int     `json:"failedTotal" yaml:"failedTotal"`
	Paused         bool    `json:"paused" yaml:"paused"`
	Timestamp      string  `json:"timestamp" yaml:"timestamp"`
}

type MonitoringTaskInfoJson struct {
	ID            string `json:"id" yaml:"id"`
	Queue         string `json:"queue" yaml:"queue"`
	Type          string `json:"type" yaml:"type"`
	Payload       string `json:"payload" yaml:"payload"`
	State         string `json:"state" yaml:"state"`
	MaxRetry      int    `json:"maxRetry" yaml:"maxRetry"`
	Retried       int    `json:"retried" yaml:"retried"`
	LastErr       string `json:"lastErr,omitempty" yaml:"lastErr,omitempty"`
	LastFailedAt  string `json:"lastFailedAt,omitempty" yaml:"lastFailedAt,omitempty"`
	NextProcessAt string `json:"nextProcessAt,omitempty" yaml:"nextProcessAt,omitempty"`
	CompletedAt   string `json:"completedAt,omitempty" yaml:"completedAt,omitempty"`
	IsOrphaned    bool   `json:"isOrphaned,omitempty" yaml:"isOrphaned,omitempty"`
	Group         string `json:"group,omitempty" yaml:"group,omitempty"`
}

type DailyStatsJson struct {
	Queue     string `json:"queue" yaml:"queue"`
	Processed int    `json:"processed" yaml:"processed"`
	Failed    int    `json:"failed" yaml:"failed"`
	Date      string `json:"date" yaml:"date"`
}

type WorkerInfoJson struct {
	TaskID   string `json:"taskId" yaml:"taskId"`
	TaskType string `json:"taskType" yaml:"taskType"`
	Queue    string `json:"queue" yaml:"queue"`
	Started  string `json:"started" yaml:"started"`
	Deadline string `json:"deadline" yaml:"deadline"`
}

type ServerInfoJson struct {
	ID             string            `json:"id" yaml:"id"`
	Host           string            `json:"host" yaml:"host"`
	PID            int               `json:"pid" yaml:"pid"`
	Concurrency    int               `json:"concurrency" yaml:"concurrency"`
	Queues         map[string]int    `json:"queues" yaml:"queues"`
	StrictPriority bool              `json:"strictPriority" yaml:"strictPriority"`
	Started        string            `json:"started" yaml:"started"`
	Status         string            `json:"status" yaml:"status"`
	ActiveWorkers  []*WorkerInfoJson `json:"activeWorkers" yaml:"activeWorkers"`
}

type SchedulerEntryJson struct {
	ID       string `json:"id" yaml:"id"`
	Spec     string `json:"spec" yaml:"spec"`
	TaskType string `json:"taskType" yaml:"taskType"`
	Next     string `json:"next" yaml:"next"`
	Prev     string `json:"prev,omitempty" yaml:"prev,omitempty"`
}

type BulkActionResponseJson struct {
	AffectedCount int `json:"affectedCount" yaml:"affectedCount"`
}

type ListQueuesResponseJson struct {
	Items []*QueueInfoJson `json:"items" yaml:"items"`
}

type ListMonitoringTasksResponseJson struct {
	Items  []*MonitoringTaskInfoJson `json:"items" yaml:"items"`
	Cursor string                    `json:"cursor,omitempty" yaml:"cursor,omitempty"`
}

type ListServersResponseJson struct {
	Items []*ServerInfoJson `json:"items" yaml:"items"`
}

type ListSchedulerEntriesResponseJson struct {
	Items []*SchedulerEntryJson `json:"items" yaml:"items"`
}

type ListQueueHistoryResponseJson struct {
	Items []*DailyStatsJson `json:"items" yaml:"items"`
}

type WorkflowInstanceJson struct {
	InstanceID  string                `json:"instanceId" yaml:"instanceId"`
	ExecutionID string                `json:"executionId" yaml:"executionId"`
	Parent      *WorkflowInstanceJson `json:"parent,omitempty" yaml:"parent,omitempty"`
}

type WorkflowInstanceRefJson struct {
	Instance    *WorkflowInstanceJson `json:"instance,omitempty" yaml:"instance,omitempty"`
	CreatedAt   time.Time             `json:"createdAt,omitempty" yaml:"createdAt,omitempty"`
	CompletedAt *time.Time            `json:"completedAt,omitempty" yaml:"completedAt,omitempty"`
	State       string                `json:"state" yaml:"state"`
	Queue       string                `json:"queue" yaml:"queue"`
}

type WorkflowHistoryEventJson struct {
	ID              string      `json:"id,omitempty" yaml:"id,omitempty"`
	SequenceID      int64       `json:"sequenceId,omitempty" yaml:"sequenceId,omitempty"`
	Type            string      `json:"type,omitempty" yaml:"type,omitempty"`
	Timestamp       time.Time   `json:"timestamp,omitempty" yaml:"timestamp,omitempty"`
	ScheduleEventID int64       `json:"scheduleEventId,omitempty" yaml:"scheduleEventId,omitempty"`
	Attributes      interface{} `json:"attributes,omitempty" yaml:"attributes,omitempty"`
	VisibleAt       *time.Time  `json:"visibleAt,omitempty" yaml:"visibleAt,omitempty"`
}

type WorkflowInstanceInfoJson struct {
	*WorkflowInstanceRefJson

	History []*WorkflowHistoryEventJson `json:"history,omitempty" yaml:"history,omitempty"`
}

type WorkflowInstanceTreeJson struct {
	*WorkflowInstanceRefJson

	WorkflowName string                      `json:"workflowName,omitempty" yaml:"workflowName,omitempty"`
	Error        bool                        `json:"error,omitempty" yaml:"error,omitempty"`
	Children     []*WorkflowInstanceTreeJson `json:"children,omitempty" yaml:"children,omitempty"`
}

type ListWorkflowInstancesResponseJson struct {
	Items  []*WorkflowInstanceRefJson `json:"items" yaml:"items"`
	Cursor string                     `json:"cursor,omitempty" yaml:"cursor,omitempty"`
}

type ListWorkflowHistoryResponseJson struct {
	Items []*WorkflowHistoryEventJson `json:"items" yaml:"items"`
}
