package connectors

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apjs"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/stretchr/testify/require"
)

// TestMigrationHookValidationCompilesOnly accepts runtime-dependent statements
// without invoking them, validating their syntax rather than their results.
func TestMigrationHookValidationCompilesOnly(t *testing.T) {
	// Even a library that would throw on initialization is not run by hook validation.
	library, err := apjs.CompileLibrary(`throw new Error("privateLibrary");`)
	require.NoError(t, err)
	for name, source := range map[string]string{
		"runtime inputs":     `({config: {set: {workspaceId: cfg.workspace_id}}, labels: {set: {team: labels.team}}})`,
		"helper call":        `migrationHelper(cfg, labels, annotations)`,
		"throw":              `throw new Error("privateRuntime");`,
		"loop":               `for (;;) {}`,
		"non-strict":         `with (cfg) { assignedWithoutDeclaration = workspace_id; }`,
		"unsupported result": `new Date()`,
	} {
		t.Run(name, func(t *testing.T) {
			// Both standalone and library-aware entry points retain compile-only semantics.
			hook := &MigrationHook{Javascript: source}
			vc := &common.ValidationContext{Path: "definition.migrations.up"}
			require.NoError(t, hook.Validate(vc))
			require.NoError(t, hook.ValidateWithJavascript(vc, library))
		})
	}

	// Optional directions may be omitted; supplying one does not require the other.
	require.NoError(t, (&Migrations{}).Validate(&common.ValidationContext{}))
	require.NoError(t, (&Migrations{Up: &MigrationHook{Javascript: `cfg.runtimeField`}}).Validate(&common.ValidationContext{}))
}

// TestMigrationHookValidationFailures keeps each field's error useful without
// disclosing source fragments or compiler messages from authored JavaScript.
func TestMigrationHookValidationFailures(t *testing.T) {
	for name, test := range map[string]struct {
		source string
		want   string
	}{
		"required":     {want: "migration javascript is required"},
		"whitespace":   {source: " \n\t", want: jsruntime.ErrInvalidSource.Error()},
		"syntax":       {source: "\nconst privateSource = ;", want: jsruntime.ErrInvalidSource.Error() + " at line 2"},
		"compiler":     {source: "let privateSource = 1;\nlet privateSource = 2;", want: jsruntime.ErrInvalidSource.Error() + " at line 2"},
		"invalid UTF8": {source: "({privateSource: '\xff'})", want: jsruntime.ErrInvalidSource.Error()},
		"source limit": {source: strings.Repeat(" ", apjs.ConnectorMigrationLimits().MaxSourceBytes+1), want: jsruntime.ErrLimitExceeded.Error()},
	} {
		t.Run(name, func(t *testing.T) {
			// The schema adapter adds its path while preserving only safe validation details.
			err := (&MigrationHook{Javascript: test.source}).Validate(&common.ValidationContext{Path: "definition.migrations.up"})
			require.ErrorContains(t, err, "definition.migrations.up.javascript:")
			require.ErrorContains(t, err, test.want)
			require.NotContains(t, err.Error(), "privateSource")
			require.NotContains(t, err.Error(), "Unexpected token")
		})
	}
}

// TestMigrationValidationAggregatesDirections retains independent up/down
// errors and passes caller cancellation through both nested field contexts.
func TestMigrationValidationAggregatesDirections(t *testing.T) {
	// A missing up hook body and invalid down syntax must both reach the caller.
	migrations := &Migrations{Up: &MigrationHook{}, Down: &MigrationHook{Javascript: `const privateSource = ;`}}
	err := migrations.Validate(&common.ValidationContext{Path: "definition.migrations"})
	var combined *multierror.Error
	require.ErrorAs(t, err, &combined)
	require.Len(t, combined.Errors, 2)
	require.ErrorContains(t, combined.Errors[0], "definition.migrations.up.javascript: migration javascript is required")
	require.ErrorContains(t, combined.Errors[1], "definition.migrations.down.javascript: invalid migration javascript expression:")

	// Cancellation wins over syntax/source-limit failures for every nonempty hook.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	migrations.Up.Javascript = strings.Repeat(" ", apjs.ConnectorMigrationLimits().MaxSourceBytes+1)
	err = migrations.Validate(&common.ValidationContext{Path: "definition.migrations", Ctx: ctx})
	require.ErrorAs(t, err, &combined)
	require.Len(t, combined.Errors, 2)
	for i, direction := range []string{"up", "down"} {
		require.ErrorContains(t, combined.Errors[i], "definition.migrations."+direction+".javascript:")
		require.ErrorContains(t, combined.Errors[i], context.Canceled.Error())
		require.NotContains(t, combined.Errors[i].Error(), jsruntime.ErrInvalidSource.Error())
		require.NotContains(t, combined.Errors[i].Error(), jsruntime.ErrLimitExceeded.Error())
	}
}

// TestConnectorMigrationValidationCombinedSource charges raw library bytes
// against each hook independently, including otherwise ignored whitespace.
func TestConnectorMigrationValidationCombinedSource(t *testing.T) {
	const source = `({})`
	limit := apjs.ConnectorMigrationLimits().MaxSourceBytes
	connector := ConnectorDefinition{
		Javascript: strings.Repeat(" ", limit-len(source)),
		Migrations: &Migrations{Up: &MigrationHook{Javascript: source}, Down: &MigrationHook{Javascript: source}},
	}
	vc := &common.ValidationContext{Path: "definition"}

	// Each direction exactly fits with the library; up and down are not summed together.
	require.NoError(t, connector.Validate(vc))
	connector.Javascript += " "
	err := connector.Validate(vc)
	var combined *multierror.Error
	require.ErrorAs(t, err, &combined)
	require.Len(t, combined.Errors, 2)
	for i, direction := range []string{"up", "down"} {
		require.ErrorContains(t, combined.Errors[i], "definition.migrations."+direction+".javascript:")
		require.ErrorContains(t, combined.Errors[i], jsruntime.ErrLimitExceeded.Error())
	}
	require.NotContains(t, err.Error(), "invalid connector javascript")

	// Full connector validation still compiles hooks when library validation has failed.
	connector.Javascript = `const privateLibrary = ;`
	connector.Migrations.Up.Javascript = `const privateHook = ;`
	connector.Migrations.Down.Javascript = `helperThatWillExistAtRuntime(cfg)`
	err = connector.Validate(vc)
	require.ErrorContains(t, err, "definition.javascript: invalid connector javascript:")
	require.ErrorContains(t, err, "definition.migrations.up.javascript: invalid migration javascript expression:")
	require.NotContains(t, err.Error(), "definition.migrations.down.javascript")
	require.NotContains(t, err.Error(), "private")
}
