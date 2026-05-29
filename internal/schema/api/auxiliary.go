package api

import (
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
)

// SessionInitiateParams is the request body for POST /session/_initiate.
// ReturnToUrl is where the browser should land after host authentication.
type SessionInitiateParams struct {
	ReturnToUrl string `json:"return_to_url" yaml:"return_to_url" example:"https://example.com/return"`
}

// SessionInitiateFailureResponse tells the SPA where to redirect when the
// current request cannot establish a session yet.
type SessionInitiateFailureResponse struct {
	RedirectUrl string `json:"redirect_url" yaml:"redirect_url" example:"https://example.com/auth"`
}

// SessionInitiateSuccessResponse is returned once a session already exists or
// has been established from the request authentication.
type SessionInitiateSuccessResponse struct {
	ActorId apid.ID `json:"actor_id" yaml:"actor_id" swaggertype:"string" example:"act_test550e8400abcde"`
}

type KeyValueJson struct {
	Key   string `json:"key" yaml:"key" example:"env"`
	Value string `json:"value" yaml:"value" example:"production"`
}

type PutKeyValueRequestJson struct {
	Value string `json:"value" yaml:"value" example:"production"`
}

// RequestEventJson documents the public request-event record projection.
type RequestEventJson struct {
	Namespace           string                  `json:"namespace" yaml:"namespace" example:"root.acme"`
	Type                string                  `json:"type" yaml:"type" example:"proxy"`
	RequestId           apid.ID                 `json:"request_id" yaml:"request_id" swaggertype:"string" example:"req_test550e8400abcde"`
	CorrelationId       string                  `json:"correlation_id,omitempty" yaml:"correlation_id,omitempty"`
	Timestamp           time.Time               `json:"timestamp" yaml:"timestamp"`
	MillisecondDuration int64                   `json:"duration" yaml:"duration" example:"150"`
	ConnectionId        apid.ID                 `json:"connection_id,omitempty" yaml:"connection_id,omitempty" swaggertype:"string"`
	ConnectorId         apid.ID                 `json:"connector_id,omitempty" yaml:"connector_id,omitempty" swaggertype:"string"`
	ConnectorVersion    uint64                  `json:"connector_version,omitempty" yaml:"connector_version,omitempty"`
	Method              string                  `json:"method" yaml:"method" example:"GET"`
	Host                string                  `json:"host" yaml:"host" example:"api.example.com"`
	Scheme              string                  `json:"scheme" yaml:"scheme" example:"https"`
	Path                string                  `json:"path" yaml:"path" example:"/v1/users"`
	RequestHttpVersion  string                  `json:"request_http_version,omitempty" yaml:"request_http_version,omitempty"`
	RequestSizeBytes    int64                   `json:"request_size_bytes,omitempty" yaml:"request_size_bytes,omitempty"`
	RequestMimeType     string                  `json:"request_mime_type,omitempty" yaml:"request_mime_type,omitempty"`
	RequestBodySkipped  string                  `json:"request_body_skipped,omitempty" yaml:"request_body_skipped,omitempty"`
	ResponseStatusCode  int                     `json:"response_status_code,omitempty" yaml:"response_status_code,omitempty" example:"200"`
	ResponseError       string                  `json:"response_error,omitempty" yaml:"response_error,omitempty"`
	ResponseHttpVersion string                  `json:"response_http_version,omitempty" yaml:"response_http_version,omitempty"`
	ResponseSizeBytes   int64                   `json:"response_size_bytes,omitempty" yaml:"response_size_bytes,omitempty"`
	ResponseMimeType    string                  `json:"response_mime_type,omitempty" yaml:"response_mime_type,omitempty"`
	ResponseBodySkipped string                  `json:"response_body_skipped,omitempty" yaml:"response_body_skipped,omitempty"`
	InternalTimeout     bool                    `json:"internal_timeout,omitempty" yaml:"internal_timeout,omitempty"`
	RequestCancelled    bool                    `json:"request_cancelled,omitempty" yaml:"request_cancelled,omitempty"`
	FullRequestRecorded bool                    `json:"full_request_recorded,omitempty" yaml:"full_request_recorded,omitempty"`
	Labels              map[string]string       `json:"labels,omitempty" yaml:"labels,omitempty"`
	ResponseSource      string                  `json:"response_source,omitempty" yaml:"response_source,omitempty" example:"upstream"`
	RateLimitId         apid.ID                 `json:"rate_limit_id,omitempty" yaml:"rate_limit_id,omitempty" swaggertype:"string"`
	RateLimitMode       string                  `json:"rate_limit_mode,omitempty" yaml:"rate_limit_mode,omitempty"`
	RateLimitBucket     map[string]string       `json:"rate_limit_bucket,omitempty" yaml:"rate_limit_bucket,omitempty"`
	RateLimitMatched    []RequestEventRateLimit `json:"rate_limit_matched,omitempty" yaml:"rate_limit_matched,omitempty"`
}

type RequestEventRateLimit struct {
	Id     apid.ID           `json:"id" yaml:"id" swaggertype:"string" example:"rl_test550e8400abcde"`
	Mode   string            `json:"mode" yaml:"mode" example:"enforce"`
	Bucket map[string]string `json:"bucket,omitempty" yaml:"bucket,omitempty"`
}

type ListRequestEventsResponseJson struct {
	Items  []*RequestEventJson `json:"items" yaml:"items"`
	Cursor string              `json:"cursor,omitempty" yaml:"cursor,omitempty"`
	Total  *int64              `json:"total,omitempty" yaml:"total,omitempty"`
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
	UpdatedAt *time.Time `json:"updated_at,omitempty" yaml:"updated_at,omitempty"`
}

type QueueInfoJson struct {
	Queue          string  `json:"queue" yaml:"queue"`
	MemoryUsage    int64   `json:"memory_usage" yaml:"memory_usage"`
	Latency        float64 `json:"latency_seconds" yaml:"latency_seconds"`
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
	ProcessedTotal int     `json:"processed_total" yaml:"processed_total"`
	FailedTotal    int     `json:"failed_total" yaml:"failed_total"`
	Paused         bool    `json:"paused" yaml:"paused"`
	Timestamp      string  `json:"timestamp" yaml:"timestamp"`
}

type MonitoringTaskInfoJson struct {
	ID            string `json:"id" yaml:"id"`
	Queue         string `json:"queue" yaml:"queue"`
	Type          string `json:"type" yaml:"type"`
	Payload       string `json:"payload" yaml:"payload"`
	State         string `json:"state" yaml:"state"`
	MaxRetry      int    `json:"max_retry" yaml:"max_retry"`
	Retried       int    `json:"retried" yaml:"retried"`
	LastErr       string `json:"last_err,omitempty" yaml:"last_err,omitempty"`
	LastFailedAt  string `json:"last_failed_at,omitempty" yaml:"last_failed_at,omitempty"`
	NextProcessAt string `json:"next_process_at,omitempty" yaml:"next_process_at,omitempty"`
	CompletedAt   string `json:"completed_at,omitempty" yaml:"completed_at,omitempty"`
	IsOrphaned    bool   `json:"is_orphaned,omitempty" yaml:"is_orphaned,omitempty"`
	Group         string `json:"group,omitempty" yaml:"group,omitempty"`
}

type DailyStatsJson struct {
	Queue     string `json:"queue" yaml:"queue"`
	Processed int    `json:"processed" yaml:"processed"`
	Failed    int    `json:"failed" yaml:"failed"`
	Date      string `json:"date" yaml:"date"`
}

type WorkerInfoJson struct {
	TaskID   string `json:"task_id" yaml:"task_id"`
	TaskType string `json:"task_type" yaml:"task_type"`
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
	StrictPriority bool              `json:"strict_priority" yaml:"strict_priority"`
	Started        string            `json:"started" yaml:"started"`
	Status         string            `json:"status" yaml:"status"`
	ActiveWorkers  []*WorkerInfoJson `json:"active_workers" yaml:"active_workers"`
}

type SchedulerEntryJson struct {
	ID       string `json:"id" yaml:"id"`
	Spec     string `json:"spec" yaml:"spec"`
	TaskType string `json:"task_type" yaml:"task_type"`
	Next     string `json:"next" yaml:"next"`
	Prev     string `json:"prev,omitempty" yaml:"prev,omitempty"`
}

type BulkActionResponseJson struct {
	AffectedCount int `json:"affected_count" yaml:"affected_count"`
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
