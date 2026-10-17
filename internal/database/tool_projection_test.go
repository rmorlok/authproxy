package database

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/stretchr/testify/require"
)

// TestToolProjectionLifecycle stores description and canonical verbs alongside
// each active revision, without retaining or sorting caller-owned slices.
func TestToolProjectionLifecycle(t *testing.T) {
	ctx, db, base, definition := standaloneToolForTest(t)
	tool := *base
	tool.Id = apid.New(apid.PrefixTool)
	tool.Name = "projected"
	tool.Description = "Lookup an account.\nReturns its current status."
	input := []string{"write", "read details", "Read"}
	tool.Verbs = input
	_, err := db.CreateStandaloneTool(ctx, &tool, definition.EncryptedDefinition, definition.DefinitionDigest)
	require.NoError(t, err)
	require.Equal(t, []string{"write", "read details", "Read"}, input)
	require.Equal(t, []string{"Read", "read details", "write"}, tool.Verbs)
	input[0] = "external mutation"
	require.Equal(t, []string{"Read", "read details", "write"}, tool.Verbs)
	stored, err := db.GetTool(ctx, tool.Id)
	require.NoError(t, err)
	require.Equal(t, tool.Description, stored.Description)
	require.Equal(t, tool.Verbs, stored.Verbs)

	input = []string{"search", "inspect"}
	stored.Description = "Search all matching accounts."
	stored.Verbs = input
	digest := strings.Repeat("a", 64)
	_, err = db.UpdateStandaloneTool(ctx, stored, 1, definition.EncryptedDefinition, digest)
	require.NoError(t, err)
	require.Equal(t, []string{"search", "inspect"}, input)
	input[0] = "external mutation"
	require.Equal(t, []string{"inspect", "search"}, stored.Verbs)
	current, err := db.GetTool(ctx, tool.Id)
	require.NoError(t, err)
	require.Equal(t, stored.Description, current.Description)
	require.Equal(t, []string{"inspect", "search"}, current.Verbs, "replacement removes previous verbs")
	require.Equal(t, uint64(2), current.ActiveRevision)
	revision, err := db.GetCurrentToolRevision(ctx, tool.Id)
	require.NoError(t, err)
	require.Equal(t, digest, revision.DefinitionDigest)
	previous, err := db.GetToolRevision(ctx, tool.Id, 1)
	require.NoError(t, err)
	require.Equal(t, definition.DefinitionDigest, previous.DefinitionDigest)
}

// TestToolProjectionValidation rejects incomplete or ambiguous discovery fields
// before inserting an identity or appending history, preserving each candidate.
func TestToolProjectionValidation(t *testing.T) {
	ctx, db, tool, definition := standaloneToolForTest(t)
	for _, test := range []struct {
		name   string
		change func(*Tool)
	}{
		{"missing description", func(v *Tool) { v.Description = "" }},
		{"blank description", func(v *Tool) { v.Description = " \t\n" }},
		{"nil verbs", func(v *Tool) { v.Verbs = nil }},
		{"empty verbs", func(v *Tool) { v.Verbs = []string{} }},
		{"empty verb", func(v *Tool) { v.Verbs = []string{"read", ""} }},
		{"blank verb", func(v *Tool) { v.Verbs = []string{"read", "\t"} }},
		{"leading space", func(v *Tool) { v.Verbs = []string{"read", " write"} }},
		{"trailing space", func(v *Tool) { v.Verbs = []string{"read", "write\n"} }},
		{"duplicate verb", func(v *Tool) { v.Verbs = []string{"write", "read", "write"} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := *tool
			test.change(&candidate)
			before := candidate
			before.Verbs = slices.Clone(candidate.Verbs)
			require.Error(t, candidate.Validate())
			_, err := db.UpdateStandaloneTool(ctx, &candidate, 1, definition.EncryptedDefinition, definition.DefinitionDigest)
			require.Error(t, err)
			require.Equal(t, before, candidate)
			candidate.Id = apid.New(apid.PrefixTool)
			candidate.Name = "invalid-projection"
			before = candidate
			before.Verbs = slices.Clone(candidate.Verbs)
			_, err = db.CreateStandaloneTool(ctx, &candidate, definition.EncryptedDefinition, definition.DefinitionDigest)
			require.Error(t, err)
			require.Equal(t, before, candidate)
			_, err = db.GetTool(ctx, candidate.Id)
			require.ErrorIs(t, err, ErrNotFound)
			require.Zero(t, countToolRevisionsForTest(t, ctx, db, candidate.Id))
		})
	}
	stored, err := db.GetTool(ctx, tool.Id)
	require.NoError(t, err)
	require.Equal(t, tool.Description, stored.Description)
	require.Equal(t, tool.Verbs, stored.Verbs)
	require.Equal(t, uint64(1), stored.ActiveRevision)
	require.Equal(t, 1, countToolRevisionsForTest(t, ctx, db, tool.Id))
}

// TestToolProjectionFailedWrites rolls back projections and newly inserted
// revisions when optimistic concurrency or live-name uniqueness rejects a write.
func TestToolProjectionFailedWrites(t *testing.T) {
	ctx, db, tool, definition := standaloneToolForTest(t)
	occupied := *tool
	occupied.Id = apid.New(apid.PrefixTool)
	occupied.Name = "occupied"
	_, err := db.CreateStandaloneTool(ctx, &occupied, definition.EncryptedDefinition, definition.DefinitionDigest)
	require.NoError(t, err)
	for _, test := range []struct {
		name     string
		expected uint64
		want     error
	}{
		{"stale", 2, ErrToolRevisionConflict},
		{"duplicate", 1, ErrDuplicate},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := *tool
			candidate.Name = occupied.Name
			candidate.Description = "Rejected replacement"
			candidate.Verbs = []string{"write", "read"}
			before := candidate
			before.Verbs = slices.Clone(candidate.Verbs)
			_, err := db.UpdateStandaloneTool(ctx, &candidate, test.expected, definition.EncryptedDefinition, strings.Repeat("b", 64))
			require.ErrorIs(t, err, test.want)
			require.Equal(t, before, candidate, "failed writes must not sort the caller's verbs")
			stored, err := db.GetTool(ctx, tool.Id)
			require.NoError(t, err)
			require.Equal(t, tool.Description, stored.Description)
			require.Equal(t, tool.Verbs, stored.Verbs)
			require.Equal(t, uint64(1), stored.ActiveRevision)
			require.Equal(t, 1, countToolRevisionsForTest(t, ctx, db, tool.Id))
		})
	}
}

// TestToolProjectionConcurrentUpdates ensures competing replacements cannot mix
// one candidate's discovery fields with the other candidate's active definition.
func TestToolProjectionConcurrentUpdates(t *testing.T) {
	ctx, db, tool, definition := standaloneToolForTest(t)
	// Allow actual overlapping PostgreSQL transactions even when the shared
	// test fixture is configured with a single connection by default.
	db.(*service).db.SetMaxOpenConns(2)
	start := make(chan struct{})
	type outcome struct {
		tool   Tool
		digest string
		err    error
	}
	results := make(chan outcome, 2)
	for _, operation := range []string{"read", "write"} {
		go func() {
			candidate := *tool
			candidate.Description = operation + " account"
			candidate.Verbs = []string{operation}
			digest := strings.Repeat("a", 64)
			if operation == "write" {
				digest = strings.Repeat("b", 64)
			}
			<-start
			_, err := db.UpdateStandaloneTool(ctx, &candidate, 1, definition.EncryptedDefinition, digest)
			results <- outcome{candidate, digest, err}
		}()
	}
	close(start)
	var winner outcome
	for range 2 {
		result := <-results
		if result.err == nil {
			require.True(t, winner.tool.Id.IsNil(), "only one competing revision can commit")
			winner = result
		} else {
			require.ErrorIs(t, result.err, ErrToolRevisionConflict)
		}
	}
	require.False(t, winner.tool.Id.IsNil())
	stored, err := db.GetTool(ctx, tool.Id)
	require.NoError(t, err)
	require.Equal(t, winner.tool.Description, stored.Description)
	require.Equal(t, winner.tool.Verbs, stored.Verbs)
	require.Equal(t, uint64(2), stored.ActiveRevision)
	current, err := db.GetCurrentToolRevision(ctx, tool.Id)
	require.NoError(t, err)
	require.Equal(t, winner.digest, current.DefinitionDigest)
	require.Equal(t, 2, countToolRevisionsForTest(t, ctx, db, tool.Id))
}

// TestToolProjectionMaintenance leaves discovery fields unchanged when parent
// label repair and encryption-key rotation update their respective columns.
func TestToolProjectionMaintenance(t *testing.T) {
	ctx, db, tool, definition := standaloneToolForTest(t)
	_, err := db.UpdateConnectionLabels(ctx, tool.ConnectionId, map[string]string{"region": "west"})
	require.NoError(t, err)
	_, err = db.ReconcileCarryForwardLabels(ctx, 1, nil)
	require.NoError(t, err)
	rotated := encfield.EncryptedField{ID: apid.New(apid.PrefixDataEncryptionKey), Data: "cm90YXRlZA=="}
	rotationTime := apctx.GetClock(ctx).Now().Add(time.Hour)
	require.NoError(t, db.BatchUpdateReEncryptedFields(apctx.WithFixedClock(ctx, rotationTime), []ReEncryptedFieldUpdate{{
		Table:            ToolRevisionsTable,
		PrimaryKeyCols:   []string{"tool_id", "revision"},
		PrimaryKeyValues: []any{tool.Id, int64(1)},
		FieldColumn:      "encrypted_definition",
		ExpectedValue:    definition.EncryptedDefinition,
		NewValue:         rotated,
	}}))
	stored, err := db.GetTool(ctx, tool.Id)
	require.NoError(t, err)
	require.Equal(t, "west", stored.Labels["apxy/cxn/region"])
	require.Equal(t, tool.Description, stored.Description)
	require.Equal(t, tool.Verbs, stored.Verbs)
	require.Equal(t, tool.ActiveRevision, stored.ActiveRevision)
	current, err := db.GetCurrentToolRevision(ctx, tool.Id)
	require.NoError(t, err)
	require.Equal(t, rotated, current.EncryptedDefinition)
	require.Equal(t, definition.DefinitionDigest, current.DefinitionDigest)
	require.Equal(t, 1, countToolRevisionsForTest(t, ctx, db, tool.Id))
}

// TestToolVerbsDBCodec covers both providers' driver representations and rejects
// malformed data without partially overwriting an already loaded value.
func TestToolVerbsDBCodec(t *testing.T) {
	original := toolVerbsDB{"read", "write"}
	value, err := original.Value()
	require.NoError(t, err)
	require.Equal(t, `["read","write"]`, value)
	for _, input := range []any{value, []byte(value.(string))} {
		var decoded toolVerbsDB
		require.NoError(t, decoded.Scan(input))
		require.Equal(t, original, decoded)
	}
	for _, empty := range []toolVerbsDB{nil, {}} {
		value, err := empty.Value()
		require.NoError(t, err)
		require.Nil(t, value)
	}
	decoded := slices.Clone(original)
	require.NoError(t, decoded.Scan(nil))
	require.Nil(t, decoded)
	for _, invalid := range []any{42, `not json`, `["read",`, `["read",1]`, `{"read":true}`} {
		decoded = slices.Clone(original)
		require.Error(t, decoded.Scan(invalid))
		require.Equal(t, original, decoded)
	}
}
