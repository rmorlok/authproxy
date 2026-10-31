package core

import (
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
)

// TestConnectionToolSetBindingResource projects pending and installed rows into
// the status contract, and both results pass response validation.
func TestConnectionToolSetBindingResource(t *testing.T) {
	updatedAt := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	row := database.ToolSetBindingListing{
		ToolSetBinding: database.ToolSetBinding{
			Id:           apid.New(apid.PrefixToolSetBinding),
			ToolSetId:    apid.New(apid.PrefixToolSet),
			ConnectionId: apid.New(apid.PrefixConnection),
			Epoch:        1,
			UpdatedAt:    updatedAt,
		},
		ConnectionNamespace: "root.acme.team",
		ToolSetNamespace:    "root.acme",
		ToolSetName:         common.ResourceName("calendar-tools"),
	}

	t.Run("pending reports no generation", func(t *testing.T) {
		resource := (&connectionToolSetBinding{row: row}).GetResource()
		require.NoError(t, resource.Validate(nil))

		require.Nil(t, resource.Status.AppliedGeneration)
		require.Equal(t, []meta.Condition{{
			Type:               schemaapi.ToolSetBindingConditionInstalled,
			Status:             meta.ConditionFalse,
			LastTransitionTime: updatedAt,
			Reason:             schemaapi.ToolSetBindingReasonInventoryPending,
		}}, resource.Status.Conditions)
	})

	t.Run("installed reports the applied generation", func(t *testing.T) {
		// Installation pins a generation and an inventory revision together.
		generation := uint64(3)
		installed := row
		installed.AppliedGeneration = &generation
		installed.InventoryRevision = 2

		resource := (&connectionToolSetBinding{row: installed}).GetResource()
		require.NoError(t, resource.Validate(nil))

		require.Equal(t, generation, *resource.Status.AppliedGeneration)
		require.Equal(t, []meta.Condition{{
			Type:               schemaapi.ToolSetBindingConditionInstalled,
			Status:             meta.ConditionTrue,
			ObservedGeneration: generation,
			LastTransitionTime: updatedAt,
			Reason:             schemaapi.ToolSetBindingReasonInventoryInstalled,
		}}, resource.Status.Conditions)

		// The response owns its generation pointer.
		*resource.Status.AppliedGeneration = 9
		require.Equal(t, uint64(3), generation)
	})
}
