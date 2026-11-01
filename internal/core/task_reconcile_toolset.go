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
	toolsetschema "github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

const taskTypeReconcileToolSet = "core:reconcile_toolset"

// toolSetBindableConnectionStates are the states that can gain a binding. A
// connection on its way out gains nothing, though one it already has stays until
// its selector stops matching, because a disconnected connection cannot execute.
var toolSetBindableConnectionStates = []database.ConnectionState{
	database.ConnectionStateSetup,
	database.ConnectionStateConfigured,
	database.ConnectionStateDisabled,
}

// reconcileToolSetTaskPayload names the ToolSet to reconcile. Everything else is
// read when the task runs, so a delayed task acts on current state.
type reconcileToolSetTaskPayload struct {
	ToolSetId apid.ID `json:"toolSetId"`
}

// newReconcileToolSetTask builds the membership task for one ToolSet.
func newReconcileToolSetTask(toolSetId apid.ID) (*asynq.Task, error) {
	payload, err := json.Marshal(reconcileToolSetTaskPayload{ToolSetId: toolSetId})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(taskTypeReconcileToolSet, payload), nil
}

// enqueueToolSetReconcile requests a membership pass after a ToolSet change.
// Failures are logged rather than failing the management request, which has
// already committed. Duplicate tasks are deliberately allowed: uniqueness could
// drop a request made while an older pass runs, and fencing makes overlap safe.
func (s *service) enqueueToolSetReconcile(ctx context.Context, toolSetId apid.ID) {
	task, err := newReconcileToolSetTask(toolSetId)
	if err != nil {
		s.logger.Error("failed to build toolset reconcile task", "toolset_id", toolSetId, "error", err)
		return
	}
	if _, err := s.ac.EnqueueContext(ctx, task); err != nil {
		s.logger.Error("failed to enqueue toolset reconcile task", "toolset_id", toolSetId, "error", err)
	}
}

// reconcileToolSetTask is the asynq handler for one ToolSet's membership pass.
func (s *service) reconcileToolSetTask(ctx context.Context, t *asynq.Task) error {
	logger := aplog.NewBuilder(s.logger).
		WithTask(t).
		WithCtx(ctx).
		Build()
	logger.Info("reconcile toolset task started")
	defer logger.Info("reconcile toolset task completed")

	var p reconcileToolSetTaskPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("%s json.Unmarshal failed: %v: %w", taskTypeReconcileToolSet, err, asynq.SkipRetry)
	}
	if p.ToolSetId.IsNil() {
		return fmt.Errorf("%s toolset id not specified: %w", taskTypeReconcileToolSet, asynq.SkipRetry)
	}

	return s.reconcileToolSet(ctx, p.ToolSetId)
}

// toolSetMembership is one pass's snapshot of a ToolSet's selection policy and
// installable inventory. plan is nil when there is nothing to install yet, in
// which case bindings stay pending.
type toolSetMembership struct {
	logical          *database.ToolSet
	namespaceMatcher string
	labelSelector    string
	labels           database.LabelSelector
	plan             *toolSetInstallPlan
}

// selects reports whether current persisted namespace and labels match. SQL only
// narrows the candidates, and this in-memory check is what decides. It compares
// namespaces literally, so `root.team_a.**` cannot match `root.teamXa.user`
// through LIKE's single-character wildcard.
func (m *toolSetMembership) selects(connection *database.Connection) bool {
	return namespace.Matches(m.namespaceMatcher, connection.Namespace) &&
		m.labels.Matches(connection.Labels)
}

// reconcileToolSet moves one live ToolSet's bindings toward its selector: it
// creates missing bindings, installs primary on pending bindings of configured
// connections, leaves installed bindings pinned, and removes nonmatching ones.
// The pass is idempotent, so a retry repeats only reads for finished work.
//
// One connection's failure does not stop the others. Failures are returned
// together so asynq retries the pass, and stale work (a selector or binding that
// changed underneath it) is rejected by storage fencing and redone from fresh
// reads. A deleted ToolSet is a no-op here; its cleanup is separate.
func (s *service) reconcileToolSet(ctx context.Context, toolSetId apid.ID) error {
	logical, err := s.db.GetToolSet(ctx, toolSetId)
	if errors.Is(err, database.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}

	membership, err := s.loadToolSetMembership(ctx, logical)
	if err != nil {
		return err
	}

	failures := &toolSetReconcileFailures{}
	if err := s.reconcileToolSetAdditions(ctx, membership, failures); err != nil {
		return err
	}
	if err := s.reconcileToolSetRemovals(ctx, membership, failures); err != nil {
		return err
	}

	return failures.err(toolSetId)
}

// loadToolSetMembership compiles the selector and the current primary. A stored
// selector or template that no longer compiles cannot be fixed by retrying.
func (s *service) loadToolSetMembership(
	ctx context.Context,
	logical *database.ToolSet,
) (*toolSetMembership, error) {
	matcher, selector, err := logical.ConnectionSelector.Compile(logical.Namespace)
	if err != nil {
		return nil, fmt.Errorf("toolset %s selector is invalid: %v: %w", logical.Id, err, asynq.SkipRetry)
	}
	labels, err := database.ParseLabelSelector(selector)
	if err != nil {
		return nil, fmt.Errorf("toolset %s label selector is invalid: %v: %w", logical.Id, err, asynq.SkipRetry)
	}
	membership := &toolSetMembership{
		logical:          logical,
		namespaceMatcher: matcher,
		labelSelector:    selector,
		labels:           labels,
	}

	primary, err := s.db.GetToolSetGenerationByState(ctx, logical.Id, toolsetschema.ToolSetReleaseStatePrimary)
	if errors.Is(err, database.ErrNotFound) {
		return membership, nil
	}
	if err != nil {
		return nil, err
	}

	view, err := s.hydrateToolSet(ctx, logical, primary)
	if err != nil {
		return nil, err
	}

	membership.plan, err = newToolSetInstallPlan(primary.Generation, &view.definition)
	switch {
	case errors.Is(err, errToolSetSourceUnsupported):
		s.logger.Warn("toolset primary has no installable source", "toolset_id", logical.Id, "generation", primary.Generation)
		return membership, nil
	case errors.Is(err, ErrInvalidArgument):
		return nil, fmt.Errorf("toolset %s generation %d no longer compiles: %v: %w", logical.Id, primary.Generation, err, asynq.SkipRetry)
	case err != nil:
		return nil, err
	}

	return membership, nil
}

// reconcileToolSetAdditions visits every selected connection that may gain a
// binding. Only a listing failure aborts; connection failures are collected.
func (s *service) reconcileToolSetAdditions(
	ctx context.Context,
	membership *toolSetMembership,
	failures *toolSetReconcileFailures,
) error {
	builder := s.db.ListConnectionsBuilder().
		WithDeletedHandling(database.DeletedHandlingExclude).
		ForStates(toolSetBindableConnectionStates).
		ForNamespaceMatcher(membership.namespaceMatcher)
	if membership.labelSelector != "" {
		builder = builder.ForLabelSelector(membership.labelSelector)
	}

	return builder.Enumerate(
		ctx,
		func(page pagination.PageResult[database.Connection]) (pagination.KeepGoing, error) {
			for i := range page.Results {
				connection := &page.Results[i]
				if !membership.selects(connection) {
					continue
				}
				if err := s.reconcileToolSetConnection(ctx, membership, connection); err != nil {
					failures.record(s, membership.logical.Id, connection.Id, err)
				}
			}
			return pagination.Continue, ctx.Err()
		},
	)
}

// reconcileToolSetConnection ensures one selected connection has a binding and
// installs the plan if that binding is pending and the connection is configured.
// An installed binding is left alone: publication never migrates it. Conflicts
// showing the connection stopped matching or became unready are not failures,
// because a later trigger or pass will handle the new state.
func (s *service) reconcileToolSetConnection(
	ctx context.Context,
	membership *toolSetMembership,
	connection *database.Connection,
) error {
	logical := membership.logical

	binding, err := s.db.GetToolSetBindingForConnection(ctx, logical.Id, connection.Id)
	if errors.Is(err, database.ErrNotFound) {
		binding, err = s.db.CreateToolSetBinding(ctx, logical.Id, connection.Id, logical.SelectorRevision)
		switch {
		case errors.Is(err, database.ErrDuplicate):
			// Another pass created it first, so continue from what that pass wrote.
			binding, err = s.db.GetToolSetBindingForConnection(ctx, logical.Id, connection.Id)
		case errors.Is(err, database.ErrNotFound),
			errors.Is(err, database.ErrToolSetBindingMembershipConflict):
			return nil
		}
	}
	if err != nil {
		return err
	}

	if binding.AppliedGeneration != nil ||
		membership.plan == nil ||
		connection.State != database.ConnectionStateConfigured {
		return nil
	}

	install, err := s.inventoryFor(ctx, membership.plan, logical, binding, connection)
	if err != nil {
		return err
	}

	_, err = s.db.InstallExplicitToolSetInventory(ctx, install)
	switch {
	case errors.Is(err, database.ErrNotFound),
		errors.Is(err, database.ErrToolSetBindingMembershipConflict),
		errors.Is(err, database.ErrToolSetInventoryConnectionNotReady):
		return nil
	default:
		return err
	}
}

// reconcileToolSetRemovals retires bindings whose connection no longer matches,
// along with their generated Tools. Storage rechecks membership against the
// captured selector revision, so a stale pass cannot remove a current binding.
// Keyset paging keeps removals from shifting the listing while it runs.
func (s *service) reconcileToolSetRemovals(
	ctx context.Context,
	membership *toolSetMembership,
	failures *toolSetReconcileFailures,
) error {
	logical := membership.logical

	return s.db.ListToolSetBindingsBuilder().
		ForToolSetId(logical.Id).
		ForToolSetNamespaceMatchers([]string{logical.Namespace}).
		Limit(database.MaxToolSetBindingListLimit).
		Enumerate(
			ctx,
			func(page pagination.PageResult[database.ToolSetBindingListing]) (pagination.KeepGoing, error) {
				for i := range page.Results {
					binding := &page.Results[i]
					if err := s.removeUnselectedToolSetBinding(ctx, membership, binding); err != nil {
						failures.record(s, logical.Id, binding.ConnectionId, err)
					}
				}
				return pagination.Continue, ctx.Err()
			},
		)
}

// removeUnselectedToolSetBinding removes one binding if its connection stopped
// matching. A connection deleted meanwhile is left for parent-deletion cleanup.
func (s *service) removeUnselectedToolSetBinding(
	ctx context.Context,
	membership *toolSetMembership,
	binding *database.ToolSetBindingListing,
) error {
	connection, err := s.db.GetConnection(ctx, binding.ConnectionId)
	if errors.Is(err, database.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if membership.selects(connection) {
		return nil
	}

	err = s.db.DeleteToolSetBinding(ctx, binding.Id, binding.Epoch, membership.logical.SelectorRevision)
	if errors.Is(err, database.ErrNotFound) ||
		errors.Is(err, database.ErrToolSetBindingMembershipConflict) {
		return nil
	}
	return err
}

// toolSetReconcileFailures counts per-connection failures and keeps the first
// error, so a pass over many connections cannot accumulate an unbounded message.
type toolSetReconcileFailures struct {
	count int
	first error
}

// record logs one connection's failure and adds it to the pass's total.
func (f *toolSetReconcileFailures) record(s *service, toolSetId, connectionId apid.ID, err error) {
	s.logger.Error("failed to reconcile toolset connection", "toolset_id", toolSetId, "connection_id", connectionId, "error", err)
	f.count++
	if f.first == nil {
		f.first = err
	}
}

// err summarizes the pass, returning nil when every connection succeeded.
func (f *toolSetReconcileFailures) err(toolSetId apid.ID) error {
	if f.count == 0 {
		return nil
	}
	return fmt.Errorf("reconciling toolset %s failed for %d connections: %w", toolSetId, f.count, f.first)
}
