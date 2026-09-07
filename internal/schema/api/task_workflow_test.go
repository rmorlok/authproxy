package api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestTaskProjectionSerializesAsResource(t *testing.T) {
	updatedAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.FixedZone("offset", 3600))
	task := NewTaskJson("opaque-token", "sync", TaskStateCompleted, &updatedAt)

	jsonBytes, err := json.Marshal(task)
	require.NoError(t, err)
	var jsonValue map[string]any
	require.NoError(t, json.Unmarshal(jsonBytes, &jsonValue))
	require.Equal(t, string(meta.APIVersionV1Alpha1), jsonValue["apiVersion"])
	require.Equal(t, string(TaskKind), jsonValue["kind"])
	require.Equal(t, "opaque-token", jsonValue["metadata"].(map[string]any)["id"])
	require.Equal(t, "2026-01-02T02:04:05Z", jsonValue["metadata"].(map[string]any)["updatedAt"])
	require.Equal(t, "sync", jsonValue["spec"].(map[string]any)["type"])
	require.Equal(t, "completed", jsonValue["status"].(map[string]any)["state"])

	yamlBytes, err := yaml.Marshal(task)
	require.NoError(t, err)
	var yamlValue map[string]any
	require.NoError(t, yaml.Unmarshal(yamlBytes, &yamlValue))
	require.Equal(t, string(meta.APIVersionV1Alpha1), yamlValue["apiVersion"])
	require.Equal(t, string(TaskKind), yamlValue["kind"])
}

func TestTaskAndWorkflowListConstructors(t *testing.T) {
	tests := []struct {
		name string
		kind meta.Kind
		data any
	}{
		{name: "queues", kind: "TaskQueueList", data: NewListTaskQueuesResponseJson(nil)},
		{name: "executions", kind: "TaskExecutionList", data: NewListTaskExecutionsResponseJson(nil, "next")},
		{name: "servers", kind: "TaskServerList", data: NewListTaskServersResponseJson(nil)},
		{name: "schedules", kind: "TaskScheduleList", data: NewListTaskSchedulesResponseJson(nil)},
		{name: "workflow instances", kind: "WorkflowInstanceList", data: NewListWorkflowInstancesResponseJson(nil, "next")},
		{name: "workflow history", kind: "WorkflowHistoryEventList", data: NewListWorkflowHistoryResponseJson(nil)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serialized, err := json.Marshal(tt.data)
			require.NoError(t, err)
			var value map[string]any
			require.NoError(t, json.Unmarshal(serialized, &value))
			require.Equal(t, string(meta.APIVersionV1Alpha1), value["apiVersion"])
			require.Equal(t, string(tt.kind), value["kind"])
			require.Empty(t, value["items"])
		})
	}
}

func TestOperationalActionConstructors(t *testing.T) {
	t.Run("task execution", func(t *testing.T) {
		action := NewTaskExecutionActionResponse(TaskExecutionRunActionKind, "default", "task-1")
		require.NoError(t, action.ValidateResponse(TaskExecutionRunActionKind))
		require.Equal(t, TaskExecutionKind, action.Metadata.Target.Kind)
		require.Equal(t, "task-1", action.Metadata.Target.ID)
		require.Equal(t, "default", action.Spec.Queue)
		require.True(t, action.Status.Succeeded)
		require.Equal(t, 1, action.Status.AffectedCount)
	})

	t.Run("queue bulk", func(t *testing.T) {
		action := NewTaskQueueActionResponse(TaskQueueRunAllActionKind, "default", "retry", 3)
		require.NoError(t, action.ValidateResponse(TaskQueueRunAllActionKind))
		require.Equal(t, TaskQueueKind, action.Metadata.Target.Kind)
		require.Equal(t, "default", action.Metadata.Target.ID)
		require.Equal(t, "retry", action.Spec.State)
		require.Equal(t, 3, action.Status.AffectedCount)
	})

	t.Run("workflow", func(t *testing.T) {
		action := NewWorkflowInstanceActionResponse(WorkflowCancelActionKind, "workflow-a", "exec-a")
		require.NoError(t, action.ValidateResponse(WorkflowCancelActionKind))
		require.Equal(t, WorkflowInstanceKind, action.Metadata.Target.Kind)
		require.Equal(t, "exec-a", action.Metadata.Target.ID)
		require.Equal(t, "workflow-a", action.Spec.InstanceID)
	})
}
