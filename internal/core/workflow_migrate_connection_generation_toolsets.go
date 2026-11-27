package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

// reconcileToolSetsAfterConnectionMigrationV1 requests installation attempts for
// pending bindings after a connector-generation migration changes their helper
// library. The generation check also handles an apply activity that committed
// its migration before failing in a later follow-up. A deleted connection or one
// that never reached this target needs no work from this migration.
//
// Listing and enqueue errors are returned so the workflow retries this activity
// independently of migration. Retrying may enqueue duplicate ToolSet tasks; the
// reconciler is idempotent and always reads current state. Installed bindings are
// left pinned to their explicitly selected ToolSet generations.
func (s *service) reconcileToolSetsAfterConnectionMigrationV1(
	ctx context.Context,
	connectionID apid.ID,
	targetGeneration uint64,
) error {
	connection, err := s.db.GetConnection(ctx, connectionID)
	if errors.Is(err, database.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load connection for toolset reconciliation after migration: %w", err)
	}
	if connection.ConnectorGeneration != targetGeneration {
		return nil
	}

	// This trusted activity can see ToolSets in parent namespaces as well as
	// the connection's own namespace; management visibility filters do not apply.
	return s.db.ListToolSetBindingsBuilder().
		ForConnectionId(connectionID).
		ForToolSetNamespaceMatchers([]string{"root.**"}).
		Enumerate(ctx, func(page pagination.PageResult[database.ToolSetBindingListing]) (pagination.KeepGoing, error) {
			for _, binding := range page.Results {
				if binding.AppliedGeneration != nil {
					continue
				}
				task, err := newReconcileToolSetTask(binding.ToolSetId)
				if err != nil {
					return pagination.Stop, err
				}
				if _, err := s.ac.EnqueueContext(ctx, task); err != nil {
					return pagination.Stop, fmt.Errorf("enqueue toolset %s reconciliation after connection migration: %w", binding.ToolSetId, err)
				}
			}
			return pagination.Continue, ctx.Err()
		})
}
