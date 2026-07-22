//go:build integration

package version_migration

import (
	"testing"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
)

func TestVersionMigrationHarnessScaffold(t *testing.T) {
	// Scenario tests live in the follow-up issues. Keep this package compiling
	// with the shared helpers while those scenarios land.
	_ = helpers.StartCoreWorkflowWorker
	_ = helpers.RequireWorkflowTaskCompleted
}
