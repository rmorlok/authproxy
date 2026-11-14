package core

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apjs"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encfield"
	encryptmock "github.com/rmorlok/authproxy/internal/encrypt/mock"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/require"
)

func TestApplyMigrationHookForGenerationSelectsUpAndDown(t *testing.T) {
	generation := NewTestConnector(cschema.ConnectorDefinition{
		Javascript: `
			function migrateUp() {
				return { config: { set: { direction: "up", from_cfg: cfg.seed } } };
			}
			function migrateDown() {
				return { config: { set: { direction: "down", from_label: labels.team } } };
			}
		`,
		Migrations: &cschema.Migrations{
			Up:   &cschema.MigrationHook{Javascript: `migrateUp()`},
			Down: &cschema.MigrationHook{Javascript: `migrateDown()`},
		},
	})

	up := newMigrationTestCandidate(t)
	up.Config["seed"] = "source"
	require.NoError(t, generation.s.applyMigrationHookForGeneration(context.Background(), up, generation, 1, 2))
	require.Equal(t, "up", up.Config["direction"])
	require.Equal(t, "source", up.Config["from_cfg"])

	down := newMigrationTestCandidate(t)
	down.UserLabels["team"] = "platform"
	require.NoError(t, generation.s.applyMigrationHookForGeneration(context.Background(), down, generation, 2, 1))
	require.Equal(t, "down", down.Config["direction"])
	require.Equal(t, "platform", down.Config["from_label"])
}

func TestApplyMigrationHookForGenerationSkipsMissingHook(t *testing.T) {
	generation := NewTestConnector(cschema.ConnectorDefinition{})
	candidate := newMigrationTestCandidate(t)

	require.NoError(t, generation.s.applyMigrationHookForGeneration(context.Background(), candidate, generation, 1, 2))
	require.Empty(t, candidate.Config)
}

// TestMigrationHookResultCompatibility preserves no-op completion values and
// ordinary JSON patches, while authored input mutations stay inside the VM.
func TestMigrationHookResultCompatibility(t *testing.T) {
	for _, source := range []string{"null", "undefined", "({})"} {
		t.Run(source, func(t *testing.T) {
			// Omitted changes leave every existing candidate value alone.
			generation := NewTestConnector(cschema.ConnectorDefinition{Migrations: &cschema.Migrations{
				Up: &cschema.MigrationHook{Javascript: source},
			}})
			candidate := newMigrationTestCandidate(t)
			candidate.Config["existing"] = true
			require.NoError(t, generation.s.applyMigrationHookForGeneration(context.Background(), candidate, generation, 1, 2))
			require.Equal(t, map[string]any{"existing": true}, candidate.Config)
		})
	}

	// Only returned patch operations apply; direct edits to copied inputs do not.
	generation := NewTestConnector(cschema.ConnectorDefinition{Migrations: &cschema.Migrations{
		Up: &cschema.MigrationHook{Javascript: `
			cfg.nested.enabled = false; labels.team = "changed"; annotations.note = "changed";
			({config: {set: {count: 1000000, fraction: 1.5, enabled: true, nested: {items: [null, "value"]}}}});
		`},
	}})
	candidate := newMigrationTestCandidate(t)
	candidate.Config["nested"] = map[string]any{"enabled": true}
	originalNested := candidate.Config["nested"].(map[string]any)
	candidate.UserLabels["team"] = "platform"
	candidate.Annotations["note"] = "original"
	require.NoError(t, generation.s.applyMigrationHookForGeneration(context.Background(), candidate, generation, 1, 2))

	// JSON decoding preserves the prior patch contract's float64 configuration numbers.
	require.Equal(t, float64(1000000), candidate.Config["count"])
	require.Equal(t, 1.5, candidate.Config["fraction"])
	require.Equal(t, true, candidate.Config["enabled"])
	require.Equal(t, map[string]any{"items": []any{nil, "value"}}, candidate.Config["nested"])
	require.Equal(t, map[string]any{"enabled": true}, originalNested)
	require.Equal(t, "platform", candidate.UserLabels["team"])
	require.Equal(t, "original", candidate.Annotations["note"])
}

// TestMigrationHookBoundsAndPrivacy checks source, input, execution, and output
// limits through the real migration caller without exposing authored details.
func TestMigrationHookBoundsAndPrivacy(t *testing.T) {
	for name, test := range map[string]struct {
		library string
		source  string
		padding bool
		want    error
	}{
		"combined source": {library: strings.Repeat(" ", apjs.ConnectorMigrationLimits().MaxSourceBytes), source: "({})", want: jsruntime.ErrLimitExceeded},
		"library syntax":  {library: "const privateSource = ;", source: "({})", want: jsruntime.ErrInvalidSource},
		"hook syntax":     {source: "privateSource ===", want: jsruntime.ErrInvalidSource},
		"input bytes":     {source: "({})", padding: true, want: jsruntime.ErrLimitExceeded},
		"execution":       {source: "for (;;) {}", want: jsruntime.ErrLimitExceeded},
		"output bytes":    {source: `({config: {set: {large: "x".repeat(1048576)}}})`, want: jsruntime.ErrLimitExceeded},
		"invalid output":  {source: `({config: {get set() { throw "privateGetter"; }}})`, want: jsruntime.ErrInvalidOutput},
		"throw after mutation": {
			source: `cfg.nested.enabled = false; labels.team = "changed"; annotations.note = "changed"; throw "privateThrow";`,
			want:   jsruntime.ErrEvaluation,
		},
	} {
		t.Run(name, func(t *testing.T) {
			generation := NewTestConnector(cschema.ConnectorDefinition{
				Javascript: test.library,
				Migrations: &cschema.Migrations{Up: &cschema.MigrationHook{Javascript: test.source}},
			})
			candidate := newMigrationTestCandidate(t)
			candidate.Config["nested"] = map[string]any{"enabled": true}
			candidate.UserLabels["team"] = "platform"
			candidate.Annotations["note"] = "original"
			if test.padding {
				candidate.Config["padding"] = strings.Repeat("x", apjs.ConnectorMigrationLimits().MaxInputBytes)
			}

			// Failed evaluation supplies no patch and cannot alter caller-owned inputs.
			err := generation.s.applyMigrationHookForGeneration(context.Background(), candidate, generation, 1, 2)
			require.ErrorIs(t, err, test.want)
			require.ErrorContains(t, err, "generation 1")
			require.NotContains(t, err.Error(), "private")
			require.Equal(t, map[string]any{"enabled": true}, candidate.Config["nested"])
			require.Equal(t, "platform", candidate.UserLabels["team"])
			require.Equal(t, "original", candidate.Annotations["note"])
			if name == "combined source" {
				// Check raw library bytes before the cache compiles even a whitespace-only program.
				require.False(t, generation.jsLoaded)
			}
		})
	}
}

// TestMigrationHookCancellation checks cancellation before loading and during
// helper or hook execution, preserving the caller's error rather than policy expiry.
func TestMigrationHookCancellation(t *testing.T) {
	for _, phase := range []string{"before load", "library", "hook"} {
		t.Run(phase, func(t *testing.T) {
			definition := cschema.ConnectorDefinition{Migrations: &cschema.Migrations{
				Up: &cschema.MigrationHook{Javascript: "({})"},
			}}
			if phase == "library" {
				definition.Javascript = "for (;;) {}"
			} else {
				definition.Migrations.Up.Javascript = "for (;;) {}"
			}
			generation := NewTestConnector(definition)
			candidate := newMigrationTestCandidate(t)
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			want := context.DeadlineExceeded
			if phase == "before load" {
				cancel()
				want = context.Canceled
			}

			// Caller cancellation ends the operation with no candidate patch.
			err := generation.s.applyMigrationHookForGeneration(ctx, candidate, generation, 1, 2)
			require.ErrorIs(t, err, want)
			require.ErrorContains(t, err, "generation 1")
			require.Empty(t, candidate.Config)
			if phase == "before load" {
				require.Nil(t, generation.def)
				require.False(t, generation.jsLoaded)
			}
		})
	}
}

// TestMigrationHookDefinitionFailure keeps failed definition loads retryable
// and distinct from authored JavaScript failures.
func TestMigrationHookDefinitionFailure(t *testing.T) {
	for _, failure := range []string{"decrypt", "decode", "canceled"} {
		t.Run(failure, func(t *testing.T) {
			definition := cschema.ConnectorDefinition{Migrations: &cschema.Migrations{
				Up: &cschema.MigrationHook{Javascript: `({config: {set: {loaded: true}}})`},
			}}
			generation := NewTestConnector(definition)
			candidate := newMigrationTestCandidate(t)
			encryption := encryptmock.NewMockE(gomock.NewController(t))
			generation.s.encrypt = encryption
			encoded, err := json.Marshal(definition)
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			loadFailure := errors.New("definition unavailable")
			encryption.EXPECT().DecryptString(ctx, generation.EncryptedDefinition).
				DoAndReturn(func(context.Context, encfield.EncryptedField) (string, error) {
					switch failure {
					case "decrypt":
						return "", loadFailure
					case "decode":
						return "{", nil
					default:
						cancel()
						return string(encoded), nil
					}
				})

			// A failed or canceled load does not become a cached library compile error.
			err = generation.s.applyMigrationHookForGeneration(ctx, candidate, generation, 1, 2)
			require.ErrorContains(t, err, "load migration hook definition for generation 1")
			require.NotErrorIs(t, err, jsruntime.ErrInvalidSource)
			switch failure {
			case "decrypt":
				require.ErrorIs(t, err, loadFailure)
			case "decode":
				var syntaxError *json.SyntaxError
				require.ErrorAs(t, err, &syntaxError)
			default:
				require.ErrorIs(t, err, context.Canceled)
			}
			require.Nil(t, generation.def)
			require.False(t, generation.jsLoaded)

			// A subsequent live call reloads successfully, and later calls reuse that definition.
			encryption.EXPECT().DecryptString(context.Background(), generation.EncryptedDefinition).Return(string(encoded), nil)
			for range 2 {
				require.NoError(t, generation.s.applyMigrationHookForGeneration(context.Background(), candidate, generation, 1, 2))
				require.Equal(t, true, candidate.Config["loaded"])
			}
		})
	}
}

// TestDecodeMigrationHookPatchNotificationSetUnset preserves notification patch
// decoding when the bounded evaluator supplies JSON directly.
func TestDecodeMigrationHookPatchNotificationSetUnset(t *testing.T) {
	// Decode the bounded JSON object directly into the existing notification contract.
	patch, err := decodeMigrationHookPatch(json.RawMessage(`{
		"notifications": {
			"set": [{"key": "heads-up", "level": "info", "title": "Heads up", "message": "Something changed."}],
			"unset": [{"key": "old-heads-up"}]
		}
	}`))
	require.NoError(t, err)
	require.Len(t, patch.Notifications.Set, 1)
	require.Equal(t, "heads-up", patch.Notifications.Set[0].Key)
	require.Len(t, patch.Notifications.Unset, 1)
	require.Equal(t, "old-heads-up", patch.Notifications.Unset[0].Key)
}

// TestDecodeMigrationHookPatchRejectsBareNotificationArray preserves the typed
// notification shape instead of accepting an unstructured array.
func TestDecodeMigrationHookPatchRejectsBareNotificationArray(t *testing.T) {
	// A JSON array still cannot replace the notification set/unset object.
	_, err := decodeMigrationHookPatch(json.RawMessage(`{
		"notifications": [{"key": "heads-up", "level": "info", "title": "Heads up", "message": "Something changed."}]
	}`))
	require.Error(t, err)
}

func TestApplyMigrationHookPatchAppliesMapSetAndUnset(t *testing.T) {
	candidate := newMigrationTestCandidate(t)
	candidate.Config = map[string]any{"keep": "yes", "remove": "old"}
	candidate.UserLabels = map[string]string{"team": "platform", "old": "gone"}
	candidate.Annotations = map[string]string{"note": "keep", "old": "gone"}

	err := applyMigrationHookPatch(candidate, migrationHookPatch{
		Config: migrationAnyPatch{
			Set:   map[string]any{"added": "value"},
			Unset: []string{"remove"},
		},
		Labels: migrationStringPatch{
			Set:   map[string]string{"tier": "gold"},
			Unset: []string{"old"},
		},
		Annotations: migrationStringPatch{
			Set:   map[string]string{"mode": "migration"},
			Unset: []string{"old"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"keep": "yes", "added": "value"}, candidate.Config)
	require.Equal(t, map[string]string{"team": "platform", "tier": "gold"}, candidate.UserLabels)
	require.Equal(t, map[string]string{"note": "keep", "mode": "migration"}, candidate.Annotations)
}

func TestApplyMigrationHookPatchRejectsInvalidLabelsAndAnnotations(t *testing.T) {
	t.Run("system label", func(t *testing.T) {
		candidate := newMigrationTestCandidate(t)
		err := applyMigrationHookPatch(candidate, migrationHookPatch{
			Labels: migrationStringPatch{
				Set: map[string]string{"apxy/cxr/type": "bad"},
			},
		})
		require.Error(t, err)
	})

	t.Run("invalid label", func(t *testing.T) {
		candidate := newMigrationTestCandidate(t)
		err := applyMigrationHookPatch(candidate, migrationHookPatch{
			Labels: migrationStringPatch{
				Set: map[string]string{"-bad": "value"},
			},
		})
		require.Error(t, err)
	})

	t.Run("invalid annotation", func(t *testing.T) {
		candidate := newMigrationTestCandidate(t)
		err := applyMigrationHookPatch(candidate, migrationHookPatch{
			Annotations: migrationStringPatch{
				Set: map[string]string{"-bad": "value"},
			},
		})
		require.Error(t, err)
	})
}

func TestApplyMigrationHookPatchKeepsHighestPriorityNotification(t *testing.T) {
	candidate := newMigrationTestCandidate(t)

	err := applyMigrationHookPatch(candidate, migrationHookPatch{
		Config: migrationAnyPatch{Set: map[string]any{"team": "platform"}},
		Notifications: migrationNotificationPatch{
			Set: []migrationNotificationDef{{
				Key:     "heads-up",
				Level:   string(database.NotificationLevelInfo),
				Title:   "Heads up",
				Message: "A migration happened.",
			}, {
				Key:     "pay-attention",
				Level:   string(database.NotificationLevelWarning),
				Title:   "Pay attention",
				Message: "A migration needs attention.",
			}},
		},
	})
	require.NoError(t, err)
	require.Equal(t, "platform", candidate.Config["team"])
	require.Len(t, candidate.Notifications, 1)
	require.Equal(t, connectionNotificationKey(candidate, "connector_notice:pay-attention"), candidate.Notifications[0].Key)
	require.Equal(t, "pay-attention", candidate.Notifications[0].Metadata["connector_notice_key"])
	require.Equal(t, candidate.Notifications[0].Key, candidate.NotificationKeys[0])
}

func TestApplyMigrationHookPatchUnsetsNotificationByKey(t *testing.T) {
	candidate := newMigrationTestCandidate(t)

	err := applyMigrationHookPatch(candidate, migrationHookPatch{
		Notifications: migrationNotificationPatch{
			Set: []migrationNotificationDef{{
				Key:     "obsolete-notice",
				Level:   string(database.NotificationLevelWarning),
				Title:   "Notice",
				Message: "A notice.",
			}},
			Unset: []migrationNotificationDef{{
				Key: "obsolete-notice",
			}},
		},
	})
	require.NoError(t, err)
	require.Empty(t, candidate.Notifications)
	require.Empty(t, candidate.NotificationKeys)
	require.Equal(t, []string{
		connectionNotificationKey(candidate, "connector_notice:obsolete-notice"),
	}, candidate.NotificationUnsetKeys)
}

func TestMigrationNotificationUpsertValidationAndPermissions(t *testing.T) {
	candidate := newMigrationTestCandidate(t)

	_, _, err := migrationNotificationUpsert(candidate, migrationNotificationDef{
		Key:     "missing-title",
		Message: "message",
	})
	require.Error(t, err)

	_, _, err = migrationNotificationUpsert(candidate, migrationNotificationDef{
		Key:     "bad-level",
		Level:   "critical",
		Title:   "Title",
		Message: "message",
	})
	require.Error(t, err)

	upsert, rank, err := migrationNotificationUpsert(candidate, migrationNotificationDef{
		Key:       "resume",
		Level:     string(database.NotificationLevelError),
		Title:     "Needs attention",
		Message:   "Review the connection.",
		ActionURL: "/connections/cxn_test?action=review",
		Metadata:  map[string]any{"from": "hook"},
	})
	require.NoError(t, err)
	require.Equal(t, migrationNotificationRankHookError, rank)
	require.Equal(t, connectionNotificationKey(candidate, "connector_notice:resume"), upsert.Key)
	require.Equal(t, "resume", upsert.Metadata["connector_notice_key"])
	require.Equal(t, "hook", upsert.Metadata["from"])
	require.NotNil(t, upsert.ActionUrl)
	require.Equal(t, "/connections/cxn_test?action=review", *upsert.ActionUrl)
	require.NotEmpty(t, upsert.ViewPermissions)
	require.NotEmpty(t, upsert.ActionPermissions)
}
