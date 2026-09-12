package core

import (
	"context"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/stretchr/testify/require"
)

func TestMigrateConnectionGenerationWorkflowInstanceID(t *testing.T) {
	connectionID := apid.New(apid.PrefixConnection)

	require.Equal(
		t,
		WorkflowNameMigrateConnectionGenerationV1+":"+connectionID.String(),
		migrateConnectionGenerationWorkflowInstanceID(connectionID),
	)
}

func TestStartMigrateConnectionGenerationWorkflowRequiresClient(t *testing.T) {
	s := &service{}

	instance, err := s.startMigrateConnectionGenerationWorkflow(context.Background(), apid.New(apid.PrefixConnection), iface.ConnectionMigrationOptions{
		TargetGeneration: 2,
		Timeout:          time.Minute,
	})
	require.Nil(t, instance)
	require.ErrorContains(t, err, "workflow client is not configured")
}
