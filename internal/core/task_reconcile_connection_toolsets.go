package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

const (
	taskTypeReconcileConnectionToolSets = "core:reconcile_connection_toolsets"
	taskTypeSweepToolSets               = "core:sweep_toolsets"

	// toolSetSweepCronspec is how often every live ToolSet is reconciled again,
	// repairing membership that a missed or failed event left behind. Changes that
	// raise no event of their own, such as labels a connection inherits from its
	// namespace or connector, also wait for this sweep.
	toolSetSweepCronspec = "@hourly"
)

// reconcileConnectionToolSetsTaskPayload names the connection whose ToolSet
// membership changed. Everything else is read when the task runs.
type reconcileConnectionToolSetsTaskPayload struct {
	ConnectionId apid.ID `json:"connectionId"`
}

// newReconcileConnectionToolSetsTask builds the membership task for one connection.
func newReconcileConnectionToolSetsTask(connectionId apid.ID) (*asynq.Task, error) {
	payload, err := json.Marshal(reconcileConnectionToolSetsTaskPayload{ConnectionId: connectionId})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(taskTypeReconcileConnectionToolSets, payload), nil
}

// enqueueConnectionToolSetReconcile requests a membership pass after a
// connection is created or its state or labels change. The change has already
// committed, so the request does not depend on the caller's context staying live,
// and a failure is logged for the periodic sweep to repair rather than failing
// the caller. Duplicate tasks are allowed for the same reasons as ToolSet passes.
func (s *service) enqueueConnectionToolSetReconcile(ctx context.Context, connectionId apid.ID) {
	task, err := newReconcileConnectionToolSetsTask(connectionId)
	if err != nil {
		s.logger.Error("failed to build connection toolset reconcile task", "connection_id", connectionId, "error", err)
		return
	}
	if _, err := s.ac.EnqueueContext(context.WithoutCancel(ctx), task); err != nil {
		s.logger.Error("failed to enqueue connection toolset reconcile task", "connection_id", connectionId, "error", err)
	}
}

// reconcileConnectionToolSetsTask is the asynq handler for one connection's pass.
func (s *service) reconcileConnectionToolSetsTask(ctx context.Context, t *asynq.Task) error {
	logger := aplog.NewBuilder(s.logger).
		WithTask(t).
		WithCtx(ctx).
		Build()
	logger.Info("reconcile connection toolsets task started")
	defer logger.Info("reconcile connection toolsets task completed")

	var p reconcileConnectionToolSetsTaskPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("%s json.Unmarshal failed: %v: %w", taskTypeReconcileConnectionToolSets, err, asynq.SkipRetry)
	}
	if p.ConnectionId.IsNil() {
		return fmt.Errorf("%s connection id not specified: %w", taskTypeReconcileConnectionToolSets, asynq.SkipRetry)
	}

	return s.reconcileConnectionToolSets(ctx, p.ConnectionId)
}

// reconcileConnectionToolSets brings one connection's bindings in line with every
// live ToolSet that could select it. A selector can only reach the ToolSet's own
// namespace and its descendants, so the candidates are the ToolSets in the
// connection's namespace and its ancestors.
//
// For each candidate it applies the same rules as a ToolSet pass: a selected,
// bindable connection gains a binding and installs the primary when configured,
// and a binding whose connection stopped matching is removed. A deleted
// connection is a no-op here; its cleanup is separate.
//
// One ToolSet's failure does not stop the others. Failures are returned together
// so asynq retries the pass, which repeats only reads for finished work.
func (s *service) reconcileConnectionToolSets(ctx context.Context, connectionId apid.ID) error {
	connection, err := s.db.GetConnection(ctx, connectionId)
	if errors.Is(err, database.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}

	failures := &toolSetReconcileFailures{}
	err = s.db.ListToolSetsBuilder().
		ForNamespaceMatchers(namespace.SplitPathToPrefixes(connection.Namespace)).
		Limit(database.MaxToolSetListLimit).
		Enumerate(
			ctx,
			func(page pagination.PageResult[database.ToolSet]) (pagination.KeepGoing, error) {
				for i := range page.Results {
					logical := &page.Results[i]
					if err := s.reconcileConnectionForToolSet(ctx, logical, connection); err != nil {
						failures.record(s, logical.Id, connection.Id, err)
					}
				}
				return pagination.Continue, ctx.Err()
			},
		)
	if err != nil {
		return err
	}

	if failures.count == 0 {
		return nil
	}
	return fmt.Errorf("reconciling connection %s failed for %d toolsets: %w", connection.Id, failures.count, failures.first)
}

// reconcileConnectionForToolSet applies one ToolSet's selector to one connection.
// A connection that is selected but not bindable, such as one being disconnected,
// keeps any binding it has, matching the ToolSet pass.
//
// A ToolSet whose stored selector or primary no longer compiles is skipped with a
// warning rather than failing the pass. Retrying cannot fix it, its own pass
// reports it, and its non-retryable error must not stop asynq from retrying other
// ToolSets' transient failures in this pass.
func (s *service) reconcileConnectionForToolSet(
	ctx context.Context,
	logical *database.ToolSet,
	connection *database.Connection,
) error {
	membership, err := s.loadToolSetMembership(ctx, logical)
	if errors.Is(err, asynq.SkipRetry) {
		s.logger.Warn("skipping toolset that cannot be reconciled", "toolset_id", logical.Id, "connection_id", connection.Id, "error", err)
		return nil
	}
	if err != nil {
		return err
	}

	if membership.selects(connection) {
		if !isToolSetBindableConnectionState(connection.State) {
			return nil
		}
		return s.reconcileToolSetConnection(ctx, membership, connection)
	}

	binding, err := s.db.GetToolSetBindingForConnection(ctx, logical.Id, connection.Id)
	if errors.Is(err, database.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.removeUnselectedToolSetBinding(ctx, membership, binding)
}

// isToolSetBindableConnectionState reports whether a connection in state may gain
// a binding. See toolSetBindableConnectionStates.
func isToolSetBindableConnectionState(state database.ConnectionState) bool {
	for _, bindable := range toolSetBindableConnectionStates {
		if state == bindable {
			return true
		}
	}
	return false
}

// newSweepToolSetsTask builds the periodic repair task. It carries no payload
// because each run covers every live ToolSet.
func newSweepToolSetsTask() *asynq.Task {
	return asynq.NewTask(taskTypeSweepToolSets, nil)
}

// sweepToolSetsTask is the asynq handler for the periodic repair sweep.
func (s *service) sweepToolSetsTask(ctx context.Context, t *asynq.Task) error {
	logger := aplog.NewBuilder(s.logger).
		WithTask(t).
		WithCtx(ctx).
		Build()
	logger.Info("sweep toolsets task started")
	defer logger.Info("sweep toolsets task completed")

	return s.sweepToolSets(ctx)
}

// sweepToolSets requests a membership pass for every live ToolSet. It only fans
// out, so each ToolSet's pass retries on its own and one large ToolSet cannot
// hold up the rest. The sweep never writes ToolSets, so offset paging over them
// cannot skip any. A failed enqueue does not stop the sweep; the failures are
// returned together so asynq retries it, and the duplicate passes a retry
// requests are harmless.
func (s *service) sweepToolSets(ctx context.Context) error {
	failed := 0
	var first error
	err := s.db.ListToolSetsBuilder().
		ForNamespaceMatcher(namespace.Root+namespace.WildcardSuffix).
		Limit(database.MaxToolSetListLimit).
		Enumerate(
			ctx,
			func(page pagination.PageResult[database.ToolSet]) (pagination.KeepGoing, error) {
				for i := range page.Results {
					task, err := newReconcileToolSetTask(page.Results[i].Id)
					if err == nil {
						_, err = s.ac.EnqueueContext(ctx, task)
					}
					if err != nil {
						s.logger.Error("failed to enqueue toolset reconcile from sweep", "toolset_id", page.Results[i].Id, "error", err)
						failed++
						if first == nil {
							first = err
						}
					}
				}
				return pagination.Continue, ctx.Err()
			},
		)
	if err != nil {
		return err
	}

	if failed > 0 {
		return fmt.Errorf("toolset sweep failed to enqueue %d toolsets: %w", failed, first)
	}
	return nil
}
