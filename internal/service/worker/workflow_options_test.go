package worker

import (
	"context"
	"testing"
	"time"

	workflowworker "github.com/cschleiden/go-workflows/worker"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/require"
)

func TestWorkflowOptionsFromConfigDefaultsToLibraryValues(t *testing.T) {
	options := workflowOptionsFromConfig(context.Background(), &config.ServiceWorker{})

	require.Equal(t, workflowworker.DefaultOptions.WorkflowPollers, options.WorkflowPollers)
	require.Equal(t, workflowworker.DefaultOptions.ActivityPollers, options.ActivityPollers)
	require.Equal(t, workflowworker.DefaultOptions.MaxParallelWorkflowTasks, options.MaxParallelWorkflowTasks)
	require.Equal(t, workflowworker.DefaultOptions.MaxParallelActivityTasks, options.MaxParallelActivityTasks)
	require.Equal(t, workflowworker.DefaultOptions.WorkflowHeartbeatInterval, options.WorkflowHeartbeatInterval)
}

func TestWorkflowOptionsFromConfigOverridesConfiguredValues(t *testing.T) {
	options := workflowOptionsFromConfig(context.Background(), &config.ServiceWorker{
		WorkflowPollers:           config.NewStringValueDirectInline("3"),
		ActivityPollers:           config.NewStringValueDirectInline("4"),
		MaxParallelWorkflowTasks:  config.NewStringValueDirectInline("5"),
		MaxParallelActivityTasks:  config.NewStringValueDirectInline("6"),
		WorkflowHeartbeatInterval: &config.HumanDuration{Duration: 7 * time.Second},
	})

	require.Equal(t, 3, options.WorkflowPollers)
	require.Equal(t, 4, options.ActivityPollers)
	require.Equal(t, 5, options.MaxParallelWorkflowTasks)
	require.Equal(t, 6, options.MaxParallelActivityTasks)
	require.Equal(t, 7*time.Second, options.WorkflowHeartbeatInterval)
}
