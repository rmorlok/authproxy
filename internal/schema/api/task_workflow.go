package api

import (
	"time"

	apiv1alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/util"
)

const (
	TaskKind                 meta.Kind = "Task"
	TaskQueueKind            meta.Kind = "TaskQueue"
	TaskQueueHistoryKind     meta.Kind = "TaskQueueHistory"
	TaskExecutionKind        meta.Kind = "TaskExecution"
	TaskServerKind           meta.Kind = "TaskServer"
	TaskScheduleKind         meta.Kind = "TaskSchedule"
	WorkflowInstanceKind     meta.Kind = "WorkflowInstance"
	WorkflowHistoryEventKind meta.Kind = "WorkflowHistoryEvent"

	TaskExecutionRunActionKind     meta.Kind = "TaskExecutionRun"
	TaskExecutionArchiveActionKind meta.Kind = "TaskExecutionArchive"
	TaskExecutionCancelActionKind  meta.Kind = "TaskExecutionCancel"
	TaskExecutionDeleteActionKind  meta.Kind = "TaskExecutionDelete"
	TaskQueuePauseActionKind       meta.Kind = "TaskQueuePause"
	TaskQueueUnpauseActionKind     meta.Kind = "TaskQueueUnpause"
	TaskQueueRunAllActionKind      meta.Kind = "TaskQueueRunAll"
	TaskQueueDeleteAllActionKind   meta.Kind = "TaskQueueDeleteAll"
	WorkflowCancelActionKind       meta.Kind = "WorkflowInstanceCancel"
	WorkflowDeleteActionKind       meta.Kind = "WorkflowInstanceDelete"
)

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

// TaskJson is the safe, read-only projection returned for an opaque task
// tracking token. The token is the projection ID and is intentionally not an
// Asynq or workflow backend identifier.
type TaskJson struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      meta.ObjectMeta `json:"metadata" yaml:"metadata"`
	Spec          TaskSpecJson    `json:"spec" yaml:"spec"`
	Status        TaskStatusJson  `json:"status" yaml:"status"`
}

type TaskSpecJson struct {
	Type string `json:"type" yaml:"type"`
}

type TaskStatusJson struct {
	State TaskState `json:"state" yaml:"state"`
}

func NewTaskJson(id, taskType string, state TaskState, updatedAt *time.Time) TaskJson {
	return TaskJson{
		TypeMeta: meta.NewTypeMeta(TaskKind),
		Metadata: meta.ObjectMeta{
			ID:        id,
			UpdatedAt: util.UtcTimePointer(updatedAt),
		},
		Spec:   TaskSpecJson{Type: taskType},
		Status: TaskStatusJson{State: state},
	}
}

// TaskQueueJson is a read-only projection of an Asynq queue. Spec is empty
// because AuthProxy exposes queue changes as imperative actions; all queue
// measurements and state are observations.
type TaskQueueJson struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      meta.ObjectMeta     `json:"metadata" yaml:"metadata"`
	Spec          TaskQueueSpecJson   `json:"spec" yaml:"spec"`
	Status        TaskQueueStatusJson `json:"status" yaml:"status"`
}

type TaskQueueSpecJson struct{}

type TaskQueueStatusJson struct {
	MemoryUsage    int64   `json:"memoryUsage" yaml:"memoryUsage"`
	LatencySeconds float64 `json:"latencySeconds" yaml:"latencySeconds"`
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
}

type ListTaskQueuesResponseJson struct {
	apiv1alpha1.ResourceList[TaskQueueJson] `json:",inline" yaml:",inline"`
}

func NewListTaskQueuesResponseJson(items []TaskQueueJson) ListTaskQueuesResponseJson {
	return ListTaskQueuesResponseJson{ResourceList: apiv1alpha1.NewResourceList(
		TaskQueueKind,
		items,
		apiv1alpha1.ListMeta{},
	)}
}

// TaskExecutionJson is an administrative projection of one Asynq task.
type TaskExecutionJson struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      meta.ObjectMeta         `json:"metadata" yaml:"metadata"`
	Spec          TaskExecutionSpecJson   `json:"spec" yaml:"spec"`
	Status        TaskExecutionStatusJson `json:"status" yaml:"status"`
}

type TaskExecutionSpecJson struct {
	Queue    string `json:"queue" yaml:"queue"`
	Type     string `json:"type" yaml:"type"`
	Payload  string `json:"payload" yaml:"payload"`
	MaxRetry int    `json:"maxRetry" yaml:"maxRetry"`
	Group    string `json:"group,omitempty" yaml:"group,omitempty"`
}

type TaskExecutionStatusJson struct {
	State         string     `json:"state" yaml:"state"`
	Retried       int        `json:"retried" yaml:"retried"`
	LastError     string     `json:"lastError,omitempty" yaml:"lastError,omitempty"`
	LastFailedAt  *time.Time `json:"lastFailedAt,omitempty" yaml:"lastFailedAt,omitempty"`
	NextProcessAt *time.Time `json:"nextProcessAt,omitempty" yaml:"nextProcessAt,omitempty"`
	CompletedAt   *time.Time `json:"completedAt,omitempty" yaml:"completedAt,omitempty"`
	Orphaned      bool       `json:"orphaned,omitempty" yaml:"orphaned,omitempty"`
}

type ListTaskExecutionsResponseJson struct {
	apiv1alpha1.ResourceList[TaskExecutionJson] `json:",inline" yaml:",inline"`
}

func NewListTaskExecutionsResponseJson(
	items []TaskExecutionJson,
	continueToken string,
) ListTaskExecutionsResponseJson {
	return ListTaskExecutionsResponseJson{ResourceList: apiv1alpha1.NewResourceList(
		TaskExecutionKind,
		items,
		apiv1alpha1.ListMeta{Continue: continueToken},
	)}
}

type TaskQueueDailyStatsJson struct {
	Date      string `json:"date" yaml:"date"`
	Processed int    `json:"processed" yaml:"processed"`
	Failed    int    `json:"failed" yaml:"failed"`
}

// TaskQueueHistoryJson is a time-series projection, not a resource list. Its
// query window is recorded in spec and the returned samples are observations.
type TaskQueueHistoryJson struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      meta.ObjectMeta            `json:"metadata" yaml:"metadata"`
	Spec          TaskQueueHistorySpecJson   `json:"spec" yaml:"spec"`
	Status        TaskQueueHistoryStatusJson `json:"status" yaml:"status"`
}

type TaskQueueHistorySpecJson struct {
	Days int `json:"days" yaml:"days"`
}

type TaskQueueHistoryStatusJson struct {
	Items []TaskQueueDailyStatsJson `json:"items" yaml:"items"`
}

type TaskWorkerJson struct {
	TaskID    string    `json:"taskId" yaml:"taskId"`
	TaskType  string    `json:"taskType" yaml:"taskType"`
	Queue     string    `json:"queue" yaml:"queue"`
	StartedAt time.Time `json:"startedAt" yaml:"startedAt"`
	Deadline  time.Time `json:"deadline" yaml:"deadline"`
}

type TaskServerJson struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      meta.ObjectMeta      `json:"metadata" yaml:"metadata"`
	Spec          TaskServerSpecJson   `json:"spec" yaml:"spec"`
	Status        TaskServerStatusJson `json:"status" yaml:"status"`
}

type TaskServerSpecJson struct {
	Host           string         `json:"host" yaml:"host"`
	PID            int            `json:"pid" yaml:"pid"`
	Concurrency    int            `json:"concurrency" yaml:"concurrency"`
	Queues         map[string]int `json:"queues" yaml:"queues"`
	StrictPriority bool           `json:"strictPriority" yaml:"strictPriority"`
}

type TaskServerStatusJson struct {
	State         string           `json:"state" yaml:"state"`
	ActiveWorkers []TaskWorkerJson `json:"activeWorkers" yaml:"activeWorkers"`
}

type ListTaskServersResponseJson struct {
	apiv1alpha1.ResourceList[TaskServerJson] `json:",inline" yaml:",inline"`
}

func NewListTaskServersResponseJson(items []TaskServerJson) ListTaskServersResponseJson {
	return ListTaskServersResponseJson{ResourceList: apiv1alpha1.NewResourceList(
		TaskServerKind,
		items,
		apiv1alpha1.ListMeta{},
	)}
}

type TaskScheduleJson struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      meta.ObjectMeta        `json:"metadata" yaml:"metadata"`
	Spec          TaskScheduleSpecJson   `json:"spec" yaml:"spec"`
	Status        TaskScheduleStatusJson `json:"status" yaml:"status"`
}

type TaskScheduleSpecJson struct {
	Schedule string `json:"schedule" yaml:"schedule"`
	TaskType string `json:"taskType" yaml:"taskType"`
}

type TaskScheduleStatusJson struct {
	NextRunAt     time.Time  `json:"nextRunAt" yaml:"nextRunAt"`
	PreviousRunAt *time.Time `json:"previousRunAt,omitempty" yaml:"previousRunAt,omitempty"`
}

type ListTaskSchedulesResponseJson struct {
	apiv1alpha1.ResourceList[TaskScheduleJson] `json:",inline" yaml:",inline"`
}

func NewListTaskSchedulesResponseJson(items []TaskScheduleJson) ListTaskSchedulesResponseJson {
	return ListTaskSchedulesResponseJson{ResourceList: apiv1alpha1.NewResourceList(
		TaskScheduleKind,
		items,
		apiv1alpha1.ListMeta{},
	)}
}

// WorkflowInstanceReferenceJson combines a typed execution reference with the
// logical workflow instance ID. metadata.id on WorkflowInstance is the unique
// execution ID; both values are needed to address the backend protocol.
type WorkflowInstanceReferenceJson struct {
	Target     meta.ObjectReference `json:"target" yaml:"target"`
	InstanceID string               `json:"instanceId" yaml:"instanceId"`
}

type WorkflowInstanceJson struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      meta.ObjectMeta            `json:"metadata" yaml:"metadata"`
	Spec          WorkflowInstanceSpecJson   `json:"spec" yaml:"spec"`
	Status        WorkflowInstanceStatusJson `json:"status" yaml:"status"`
}

type WorkflowInstanceSpecJson struct {
	InstanceID   string                         `json:"instanceId" yaml:"instanceId"`
	Queue        string                         `json:"queue" yaml:"queue"`
	ParentRef    *WorkflowInstanceReferenceJson `json:"parentRef,omitempty" yaml:"parentRef,omitempty"`
	WorkflowName string                         `json:"workflowName,omitempty" yaml:"workflowName,omitempty"`
}

type WorkflowInstanceStatusJson struct {
	State       string                     `json:"state" yaml:"state"`
	CompletedAt *time.Time                 `json:"completedAt,omitempty" yaml:"completedAt,omitempty"`
	Error       bool                       `json:"error,omitempty" yaml:"error,omitempty"`
	History     []WorkflowHistoryEventJson `json:"history,omitempty" yaml:"history,omitempty"`
	Children    []WorkflowInstanceJson     `json:"children,omitempty" yaml:"children,omitempty"`
}

type ListWorkflowInstancesResponseJson struct {
	apiv1alpha1.ResourceList[WorkflowInstanceJson] `json:",inline" yaml:",inline"`
}

func NewListWorkflowInstancesResponseJson(
	items []WorkflowInstanceJson,
	continueToken string,
) ListWorkflowInstancesResponseJson {
	return ListWorkflowInstancesResponseJson{ResourceList: apiv1alpha1.NewResourceList(
		WorkflowInstanceKind,
		items,
		apiv1alpha1.ListMeta{Continue: continueToken},
	)}
}

type WorkflowHistoryEventJson struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      meta.ObjectMeta              `json:"metadata" yaml:"metadata"`
	Spec          WorkflowHistoryEventSpecJson `json:"spec" yaml:"spec"`
}

type WorkflowHistoryEventSpecJson struct {
	SequenceID      int64       `json:"sequenceId,omitempty" yaml:"sequenceId,omitempty"`
	Type            string      `json:"type,omitempty" yaml:"type,omitempty"`
	ScheduleEventID int64       `json:"scheduleEventId,omitempty" yaml:"scheduleEventId,omitempty"`
	Attributes      interface{} `json:"attributes,omitempty" yaml:"attributes,omitempty"`
	VisibleAt       *time.Time  `json:"visibleAt,omitempty" yaml:"visibleAt,omitempty"`
}

type ListWorkflowHistoryResponseJson struct {
	apiv1alpha1.ResourceList[WorkflowHistoryEventJson] `json:",inline" yaml:",inline"`
}

func NewListWorkflowHistoryResponseJson(items []WorkflowHistoryEventJson) ListWorkflowHistoryResponseJson {
	return ListWorkflowHistoryResponseJson{ResourceList: apiv1alpha1.NewResourceList(
		WorkflowHistoryEventKind,
		items,
		apiv1alpha1.ListMeta{},
	)}
}

type TaskExecutionActionSpecJson struct {
	Queue string `json:"queue" yaml:"queue"`
}

type TaskQueueActionSpecJson struct {
	State string `json:"state,omitempty" yaml:"state,omitempty"`
}

type WorkflowInstanceActionSpecJson struct {
	InstanceID string `json:"instanceId" yaml:"instanceId"`
}

type OperationActionStatusJson struct {
	Succeeded     bool `json:"succeeded" yaml:"succeeded"`
	AffectedCount int  `json:"affectedCount" yaml:"affectedCount"`
}

type TaskExecutionActionJson struct {
	apiv1alpha1.Action[TaskExecutionActionSpecJson, OperationActionStatusJson] `json:",inline" yaml:",inline"`
}

type TaskQueueActionJson struct {
	apiv1alpha1.Action[TaskQueueActionSpecJson, OperationActionStatusJson] `json:",inline" yaml:",inline"`
}

type WorkflowInstanceActionJson struct {
	apiv1alpha1.Action[WorkflowInstanceActionSpecJson, OperationActionStatusJson] `json:",inline" yaml:",inline"`
}

func NewTaskExecutionActionResponse(kind meta.Kind, queue, taskID string) TaskExecutionActionJson {
	return TaskExecutionActionJson{Action: apiv1alpha1.NewActionResponse(
		kind,
		meta.ObjectReference{APIVersion: meta.APIVersionV1Alpha1, Kind: TaskExecutionKind, ID: taskID},
		TaskExecutionActionSpecJson{Queue: queue},
		OperationActionStatusJson{Succeeded: true, AffectedCount: 1},
	)}
}

func NewTaskQueueActionResponse(kind meta.Kind, queue, state string, affectedCount int) TaskQueueActionJson {
	return TaskQueueActionJson{Action: apiv1alpha1.NewActionResponse(
		kind,
		meta.ObjectReference{APIVersion: meta.APIVersionV1Alpha1, Kind: TaskQueueKind, ID: queue},
		TaskQueueActionSpecJson{State: state},
		OperationActionStatusJson{Succeeded: true, AffectedCount: affectedCount},
	)}
}

func NewWorkflowInstanceActionResponse(kind meta.Kind, instanceID, executionID string) WorkflowInstanceActionJson {
	return WorkflowInstanceActionJson{Action: apiv1alpha1.NewActionResponse(
		kind,
		meta.ObjectReference{APIVersion: meta.APIVersionV1Alpha1, Kind: WorkflowInstanceKind, ID: executionID},
		WorkflowInstanceActionSpecJson{InstanceID: instanceID},
		OperationActionStatusJson{Succeeded: true, AffectedCount: 1},
	)}
}
