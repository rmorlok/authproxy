package worker

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWorkerHealthAsync(t *testing.T) {
	h := &workerHealth{}

	// Before the Asynq server starts the worker is not ready.
	assert.False(t, h.asyncHealthy())

	// Once running with no reported errors it is ready.
	h.asyncRunning.Store(true)
	assert.True(t, h.asyncHealthy())

	// A failed Redis health check marks it unready...
	h.onAsyncHealthCheck(errors.New("redis down"))
	assert.False(t, h.asyncHealthy())

	// ...and the next successful check clears it rather than latching.
	h.onAsyncHealthCheck(nil)
	assert.True(t, h.asyncHealthy())

	// A failed scheduler start is reported, and a later successful start clears it.
	h.onSchedulerHealthCheck(false, errors.New("start failed"))
	assert.False(t, h.asyncHealthy())
	assert.False(t, h.asyncIsScheduler.Load())
	h.onSchedulerHealthCheck(true, nil)
	assert.True(t, h.asyncHealthy())
	assert.True(t, h.asyncIsScheduler.Load())

	// A fatal run failure latches.
	h.asyncFailed.Store(true)
	h.onAsyncHealthCheck(nil)
	assert.False(t, h.asyncHealthy())

	// After shutdown the server is no longer healthy.
	h.asyncFailed.Store(false)
	h.asyncRunning.Store(false)
	assert.False(t, h.asyncHealthy())
}

func TestWorkerHealthWorkflow(t *testing.T) {
	h := &workerHealth{}

	// Not ready until the workflow worker is running.
	assert.False(t, h.workflowHealthy())
	h.workflowRunning.Store(true)
	assert.True(t, h.workflowHealthy())

	// An exit with error latches unready.
	h.workflowFailed.Store(true)
	assert.False(t, h.workflowHealthy())
}
