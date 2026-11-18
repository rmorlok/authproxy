package connectors

import (
	"context"
	"strings"
	"testing"
	"time"

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
			// An explicit nil or compiled library retains compile-only semantics.
			hook := &MigrationHook{Javascript: source}
			vc := &common.ValidationContext{Path: "definition.migrations.up"}
			require.NoError(t, hook.Validate(vc, nil))
			require.NoError(t, hook.Validate(vc, library))
		})
	}

	// Optional directions may be omitted; supplying one does not require the other.
	require.NoError(t, (&Migrations{}).Validate(&common.ValidationContext{}, nil))
	require.NoError(t, (&Migrations{Up: &MigrationHook{Javascript: `cfg.runtimeField`}}).Validate(&common.ValidationContext{}, nil))
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
			err := (&MigrationHook{Javascript: test.source}).Validate(&common.ValidationContext{Path: "definition.migrations.up"}, nil)
			require.ErrorContains(t, err, "definition.migrations.up.javascript:")
			require.ErrorContains(t, err, test.want)
			require.NotContains(t, err.Error(), "privateSource")
			require.NotContains(t, err.Error(), "Unexpected token")
		})
	}
}

// TestMigrationValidationAggregatesDirections retains independent up/down
// definition errors while the caller still permits validation to continue.
func TestMigrationValidationAggregatesDirections(t *testing.T) {
	// A missing up hook body and invalid down syntax must both reach the caller.
	migrations := &Migrations{Up: &MigrationHook{}, Down: &MigrationHook{Javascript: `const privateSource = ;`}}
	err := migrations.Validate(&common.ValidationContext{Path: "definition.migrations"}, nil)
	var combined *multierror.Error
	require.ErrorAs(t, err, &combined)
	require.Len(t, combined.Errors, 2)
	require.ErrorContains(t, combined.Errors[0], "definition.migrations.up.javascript: migration javascript is required")
	require.ErrorContains(t, combined.Errors[1], "definition.migrations.down.javascript: invalid migration javascript expression:")
}

// TestMigrationValidationCancellation returns operational cancellation directly
// at every schema entry point, without adding invalid-syntax field errors.
func TestMigrationValidationCancellation(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		name, want := "canceled", context.Canceled
		if deadline {
			name, want = "expired", context.DeadlineExceeded
		}
		t.Run(name, func(t *testing.T) {
			// Already-stopped contexts must bypass even missing or oversized hook sources.
			ctx, cancel := context.WithCancel(context.Background())
			if deadline {
				cancel()
				ctx, cancel = context.WithDeadline(context.Background(), time.Time{})
			}
			defer cancel()
			if !deadline {
				cancel()
			}
			vc := &common.ValidationContext{Path: "definition", Ctx: ctx}
			hook := &MigrationHook{}
			require.Equal(t, want, hook.Validate(vc, nil))
			hook.Javascript = strings.Repeat(" ", apjs.ConnectorMigrationLimits().MaxSourceBytes+1)
			require.Equal(t, want, hook.Validate(vc, nil))

			// No later direction or connector phase turns cancellation into an aggregate.
			migrations := &Migrations{Up: hook, Down: &MigrationHook{Javascript: `const privateSource = ;`}}
			require.Equal(t, want, migrations.Validate(vc, nil))
			require.Equal(t, want, (&Migrations{}).Validate(vc, nil))
			connector := ConnectorDefinition{Javascript: `const privateLibrary = ;`, Migrations: migrations}
			require.Equal(t, want, connector.Validate(vc))
		})
	}
}

// cancellationValidationAuth lets the test cancel from a nested validator that
// stores its operational error as field text, as existing schema validators do.
type cancellationValidationAuth struct {
	AuthNoAuth
	// validate controls the ordinary auth phase; mustache controls the later
	// cross-field phase so tests can observe whether validation continued.
	validate func(*common.ValidationContext) error
	mustache func(*common.ValidationContext) error
}

// Validate runs the test's auth-validation action.
func (a *cancellationValidationAuth) Validate(vc *common.ValidationContext) error {
	return a.validate(vc)
}

// ValidateMustacheReferences runs the test's final cross-field validation action.
func (a *cancellationValidationAuth) ValidateMustacheReferences(vc *common.ValidationContext, _ *MustacheValidationContext) error {
	return a.mustache(vc)
}

// TestConnectorValidationCancellationPhases stops later work and preserves the
// caller's cause even when cancellation arrives inside a text-wrapping validator.
func TestConnectorValidationCancellationPhases(t *testing.T) {
	for _, finalPhase := range []bool{false, true} {
		name := "auth phase"
		if finalPhase {
			name = "final phase"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			mustacheCalls := 0
			auth := &cancellationValidationAuth{
				validate: func(vc *common.ValidationContext) error {
					if !finalPhase {
						cancel()
						return vc.NewErrorf("formatted operational error: %v", ctx.Err())
					}
					return nil
				},
				mustache: func(vc *common.ValidationContext) error {
					mustacheCalls++
					cancel()
					return vc.NewErrorf("formatted operational error: %v", ctx.Err())
				},
			}
			connector := ConnectorDefinition{
				Auth:       &Auth{InnerVal: auth},
				Migrations: &Migrations{Up: &MigrationHook{Javascript: `const privateSource = ;`}},
			}

			// Cancellation supersedes earlier definition errors instead of joining them.
			err := connector.Validate(&common.ValidationContext{Path: "definition", Ctx: ctx})
			require.Same(t, context.Canceled, err)
			if finalPhase {
				require.Equal(t, 1, mustacheCalls)
			} else {
				// Once auth validation cancels, later cross-field validation is skipped.
				require.Zero(t, mustacheCalls)
			}
		})
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
