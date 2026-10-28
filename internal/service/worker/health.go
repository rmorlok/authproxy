package worker

import "sync/atomic"

// workerHealth tracks the liveness of the worker's background subsystems (the
// Asynq server, the Asynq periodic-task scheduler, and the workflow worker) so
// the worker's /healthz endpoint can report readiness.
//
// The fields are written from the subsystems' goroutines and callbacks and read
// by the HTTP handler, so every field is atomic. Health callbacks record the
// most recent observation rather than latching the first error: a transient
// Redis failure clears once Redis recovers instead of leaving the worker unready
// until the process restarts. Fatal startup failures are latched separately
// because the process exits right after them.
type workerHealth struct {
	// asyncRunning is true while the Asynq server's Run loop is active.
	asyncRunning atomic.Bool
	// asyncFailed latches when the Asynq server or scheduler fails to run.
	asyncFailed atomic.Bool
	// asyncRedisErr is true when the Asynq server's most recent periodic Redis
	// health check failed.
	asyncRedisErr atomic.Bool
	// schedulerErr is true when the scheduler's most recent attempt to start
	// the periodic task manager failed.
	schedulerErr atomic.Bool
	// asyncIsScheduler is true while this worker holds the scheduler lock.
	asyncIsScheduler atomic.Bool

	// workflowRunning is true while the workflow worker is running.
	workflowRunning atomic.Bool
	// workflowFailed latches when the workflow worker exits with an error.
	workflowFailed atomic.Bool
}

// onAsyncHealthCheck is the Asynq server's HealthCheckFunc. Asynq calls it on
// every health-check interval with the latest Redis ping result (nil when
// healthy).
func (h *workerHealth) onAsyncHealthCheck(err error) {
	h.asyncRedisErr.Store(err != nil)
}

// onSchedulerHealthCheck records whether this worker holds the scheduler lock
// and whether starting the periodic task manager succeeded.
func (h *workerHealth) onSchedulerHealthCheck(isScheduler bool, err error) {
	h.schedulerErr.Store(err != nil)
	h.asyncIsScheduler.Store(isScheduler)
}

// asyncHealthy reports whether the Asynq server is running and its most recent
// Redis and scheduler checks succeeded.
func (h *workerHealth) asyncHealthy() bool {
	return h.asyncRunning.Load() &&
		!h.asyncFailed.Load() &&
		!h.asyncRedisErr.Load() &&
		!h.schedulerErr.Load()
}

// workflowHealthy reports whether the workflow worker is running without error.
func (h *workerHealth) workflowHealthy() bool {
	return h.workflowRunning.Load() && !h.workflowFailed.Load()
}
