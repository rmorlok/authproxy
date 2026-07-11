package core

import (
	"context"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/stretchr/testify/require"
)

func TestMigrateConnectionVersionWorkflowInstanceID(t *testing.T) {
	connectionID := apid.New(apid.PrefixConnection)

	require.Equal(
		t,
		WorkflowNameMigrateConnectionVersionV1+":"+connectionID.String(),
		migrateConnectionVersionWorkflowInstanceID(connectionID),
	)
}

func TestStartMigrateConnectionVersionWorkflowRequiresClient(t *testing.T) {
	s := &service{}

	instance, err := s.startMigrateConnectionVersionWorkflow(context.Background(), apid.New(apid.PrefixConnection), iface.ConnectionMigrationOptions{
		TargetVersion: 2,
		Timeout:       time.Minute,
	})
	require.Nil(t, instance)
	require.ErrorContains(t, err, "workflow client is not configured")
}
