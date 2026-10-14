package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/encfield"
	toolsetschema "github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
	"github.com/rmorlok/authproxy/internal/util"
)

// ToolSetGenerationsTable stores generation-owned definitions and release state.
const ToolSetGenerationsTable = "tool_set_generations"

// ErrToolSetGenerationRevisionConflict means the requested draft was edited or
// published since it was read. Callers must reload and validate before retrying.
var ErrToolSetGenerationRevisionConflict = errors.New("toolset generation is no longer the expected draft revision")

// ToolSetGeneration is an encrypted definition owned by a logical ToolSet.
// Its composite identity is retained after soft deletion, so generation numbers
// cannot be reused. Published plaintext is immutable; key rotation may replace
// its ciphertext without changing the definition revision or release state.
type ToolSetGeneration struct {
	ToolSetId           apid.ID
	Generation          uint64
	State               toolsetschema.ToolSetReleaseState
	EncryptedDefinition encfield.EncryptedField

	// Revision starts at one and advances on every accepted draft replacement.
	// Publication compares the revision the caller validated. It is independent
	// of the logical ToolSet's selector revision and of encryption-key rotation.
	Revision    uint64
	CreatedAt   time.Time
	UpdatedAt   time.Time
	EncryptedAt *time.Time
	DeletedAt   *time.Time
}

func init() {
	RegisterEncryptedField(EncryptedFieldRegistration{
		Table:            ToolSetGenerationsTable,
		PrimaryKeyCols:   []string{"tool_set_id", "generation"},
		EncryptedCols:    []string{"encrypted_definition"},
		JoinTable:        ToolSetsTable,
		JoinLocalCol:     "tool_set_id",
		JoinRemoteCol:    "id",
		JoinNamespaceCol: "namespace",
	})
}

// cols lists columns in migration order, matching fields and values.
func (g *ToolSetGeneration) cols() []string {
	return []string{
		"tool_set_id",
		"generation",
		"state",
		"encrypted_definition",
		"revision",
		"created_at",
		"updated_at",
		"encrypted_at",
		"deleted_at",
	}
}

// fields supplies destinations for scanning one encrypted generation.
func (g *ToolSetGeneration) fields() []any {
	return []any{
		&g.ToolSetId,
		&g.Generation,
		&g.State,
		&g.EncryptedDefinition,
		&g.Revision,
		&g.CreatedAt,
		&g.UpdatedAt,
		&g.EncryptedAt,
		&g.DeletedAt,
	}
}

// values supplies insertion values using the canonical encrypted-field codec.
func (g *ToolSetGeneration) values() []any {
	return []any{
		g.ToolSetId,
		g.Generation,
		g.State,
		g.EncryptedDefinition,
		g.Revision,
		g.CreatedAt,
		g.UpdatedAt,
		g.EncryptedAt,
		g.DeletedAt,
	}
}

// Validate checks storage identity, counters, release state, and the presence of
// encrypted content. Decryption and definition/compiler validation belong to the
// calling service, including source readiness checks before publication.
func (g *ToolSetGeneration) Validate() error {
	if g == nil {
		return errors.New("toolset generation is required")
	}

	if err := validateToolSetGenerationAddress(g.ToolSetId, g.Generation); err != nil {
		return err
	}

	if err := validateToolSetGenerationRevision(g.Revision); err != nil {
		return err
	}

	switch g.State {
	case toolsetschema.ToolSetReleaseStateDraft,
		toolsetschema.ToolSetReleaseStatePrimary,
		toolsetschema.ToolSetReleaseStateActive,
		toolsetschema.ToolSetReleaseStateArchived:
	default:
		return errors.New("invalid toolset generation state")
	}

	if g.EncryptedDefinition.IsZero() {
		return errors.New("encrypted toolset definition is required")
	}

	return nil
}

// validateToolSetGenerationAddress rejects identities outside the shared SQL
// integer range before they reach a database driver.
func validateToolSetGenerationAddress(id apid.ID, generation uint64) error {
	if err := validateToolSetID(id); err != nil {
		return err
	}

	if generation == 0 || generation > math.MaxInt64 {
		return errors.New("toolset generation must be a positive signed 64-bit integer")
	}

	return nil
}

// validateToolSetGenerationRevision rejects missing or unrepresentable revisions.
func validateToolSetGenerationRevision(revision uint64) error {
	if revision == 0 || revision > math.MaxInt64 {
		return errors.New("toolset generation revision must be a positive signed 64-bit integer")
	}

	return nil
}

// lockToolSetForGenerationWrite takes a metadata-neutral write lock on a live
// parent before reading any generations. This serializes allocation, draft edits,
// publication, and parent deletion on PostgreSQL and SQLite without a deferred
// SQLite read transaction needing to upgrade its snapshot to a write lock.
// The caller owns the transaction and must commit or roll it back.
func (s *service) lockToolSetForGenerationWrite(
	ctx context.Context,
	tx *sql.Tx,
	id apid.ID,
) error {
	result, err := s.sq.
		Update(ToolSetsTable).
		Set("id", id).
		Where(sq.Eq{
			"id":         id,
			"deleted_at": nil,
		}).
		RunWith(tx).
		ExecContext(ctx)
	if err != nil {
		return err
	}

	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}

	return nil
}

// CreateToolSetGeneration creates a revision-one draft under a live ToolSet.
// It allocates the next number across all historical rows, including tombstones.
// An existing live draft returns ErrDuplicate; definitions are never upserted.
func (s *service) CreateToolSetGeneration(
	ctx context.Context,
	id apid.ID,
	definition encfield.EncryptedField,
) (*ToolSetGeneration, error) {
	now := apctx.GetClock(ctx).Now()

	result := ToolSetGeneration{
		ToolSetId:           id,
		Generation:          1,
		State:               toolsetschema.ToolSetReleaseStateDraft,
		EncryptedDefinition: definition,
		Revision:            1,
		CreatedAt:           now,
		UpdatedAt:           now,
		EncryptedAt:         &now,
	}
	if err := result.Validate(); err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if err := s.lockToolSetForGenerationWrite(ctx, tx, id); err != nil {
		return nil, err
	}

	var previous uint64
	err = s.sq.
		Select("COALESCE(MAX(generation), 0)").
		From(ToolSetGenerationsTable).
		Where(sq.Eq{"tool_set_id": id}).
		RunWith(tx).
		QueryRowContext(ctx).
		Scan(&previous)
	if err != nil {
		return nil, err
	}
	if previous >= math.MaxInt64 {
		return nil, errors.New("toolset generation number exhausted")
	}
	result.Generation = previous + 1

	_, err = s.sq.
		Insert(ToolSetGenerationsTable).
		Columns(result.cols()...).
		Values(result.values()...).
		RunWith(tx).
		ExecContext(ctx)
	if err != nil {
		return nil, wrapDatabaseMutationError("failed to create toolset generation", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, wrapDatabaseMutationError("failed to commit toolset generation", err)
	}

	return &result, nil
}

// getToolSetGeneration reads one generation through a supplied runner. Conditions
// use the g alias; both the generation and its logical parent must be live.
func (s *service) getToolSetGeneration(
	ctx context.Context,
	runner sq.BaseRunner,
	where sq.Eq,
) (*ToolSetGeneration, error) {
	var result ToolSetGeneration

	columns := util.PrependStringToAll("g.", result.cols())
	err := s.sq.
		Select(columns...).
		From(ToolSetGenerationsTable + " g").
		Join(ToolSetsTable + " t ON t.id = g.tool_set_id").
		Where(where).
		Where(sq.Eq{
			"g.deleted_at": nil,
			"t.deleted_at": nil,
		}).
		RunWith(runner).
		QueryRowContext(ctx).
		Scan(result.fields()...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// GetToolSetGeneration returns a live generation by its composite identity.
func (s *service) GetToolSetGeneration(
	ctx context.Context,
	id apid.ID,
	generation uint64,
) (*ToolSetGeneration, error) {
	if err := validateToolSetGenerationAddress(id, generation); err != nil {
		return nil, err
	}

	return s.getToolSetGeneration(ctx, s.db, sq.Eq{"g.tool_set_id": id, "g.generation": generation})
}

// GetToolSetGenerationByState finds the sole draft or primary of a live ToolSet.
// Other states are rejected because multiple historical generations can share them.
func (s *service) GetToolSetGenerationByState(
	ctx context.Context,
	id apid.ID,
	state toolsetschema.ToolSetReleaseState,
) (*ToolSetGeneration, error) {
	if err := validateToolSetID(id); err != nil {
		return nil, err
	}

	if state != toolsetschema.ToolSetReleaseStateDraft &&
		state != toolsetschema.ToolSetReleaseStatePrimary {
		return nil, errors.New("toolset generation lookup requires draft or primary state")
	}

	return s.getToolSetGeneration(ctx, s.db, sq.Eq{"g.tool_set_id": id, "g.state": state})
}

// UpdateToolSetGenerationDraft replaces only the observed editable revision.
// Every accepted replacement advances Revision, even for identical content.
// Published definitions cannot be edited; stale or exhausted revisions conflict.
func (s *service) UpdateToolSetGenerationDraft(
	ctx context.Context,
	id apid.ID,
	generation uint64,
	expectedRevision uint64,
	definition encfield.EncryptedField,
) (*ToolSetGeneration, error) {
	if err := validateToolSetGenerationAddress(id, generation); err != nil {
		return nil, err
	}

	if err := validateToolSetGenerationRevision(expectedRevision); err != nil {
		return nil, err
	}

	if definition.IsZero() {
		return nil, errors.New("encrypted toolset definition is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if err := s.lockToolSetForGenerationWrite(ctx, tx, id); err != nil {
		return nil, err
	}
	current, err := s.getToolSetGeneration(ctx, tx, sq.Eq{
		"g.tool_set_id": id,
		"g.generation":  generation,
	})
	if err != nil {
		return nil, err
	}

	if current.State != toolsetschema.ToolSetReleaseStateDraft ||
		current.Revision != expectedRevision ||
		expectedRevision == math.MaxInt64 {
		return nil, ErrToolSetGenerationRevisionConflict
	}

	now := apctx.GetClock(ctx).Now()
	var result ToolSetGeneration
	err = s.sq.
		Update(ToolSetGenerationsTable).
		Set("encrypted_definition", definition).
		Set("revision", sq.Expr("revision + 1")).
		Set("updated_at", now).
		Set("encrypted_at", now).
		Where(sq.Eq{
			"tool_set_id": id,
			"generation":  generation,
			"state":       toolsetschema.ToolSetReleaseStateDraft,
			"revision":    expectedRevision,
			"deleted_at":  nil,
		}).
		Suffix("RETURNING " + strings.Join(result.cols(), ", ")).
		RunWith(tx).
		QueryRowContext(ctx).
		Scan(result.fields()...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrToolSetGenerationRevisionConflict
	}
	if err != nil {
		return nil, fmt.Errorf("failed to update toolset draft: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit toolset draft: %w", err)
	}

	return &result, nil
}

// PublishToolSetGeneration promotes exactly the draft revision validated by the
// caller and demotes the previous primary to active in the same transaction.
// Definition/source validation must finish before this call; it performs no
// decryption or compilation. Publication leaves content revisions, ciphertext,
// selector policy, and existing connection bindings unchanged.
func (s *service) PublishToolSetGeneration(
	ctx context.Context,
	id apid.ID,
	generation uint64,
	expectedRevision uint64,
) (*ToolSetGeneration, error) {
	if err := validateToolSetGenerationAddress(id, generation); err != nil {
		return nil, err
	}

	if err := validateToolSetGenerationRevision(expectedRevision); err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if err := s.lockToolSetForGenerationWrite(ctx, tx, id); err != nil {
		return nil, err
	}

	current, err := s.getToolSetGeneration(ctx, tx, sq.Eq{
		"g.tool_set_id": id,
		"g.generation": generation,
	})
	if err != nil {
		return nil, err
	}
	if current.State != toolsetschema.ToolSetReleaseStateDraft || current.Revision != expectedRevision {
		return nil, ErrToolSetGenerationRevisionConflict
	}

	// Release the unique primary slot before promoting the draft. Rollback
	// restores the former primary if promotion or commit fails.
	now := apctx.GetClock(ctx).Now()
	_, err = s.sq.
		Update(ToolSetGenerationsTable).
		Set("state", toolsetschema.ToolSetReleaseStateActive).
		Set("updated_at", now).
		Where(sq.Eq{
			"tool_set_id": id,
			"state":       toolsetschema.ToolSetReleaseStatePrimary,
			"deleted_at":  nil,
		}).
		RunWith(tx).
		ExecContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to demote previous toolset primary: %w", err)
	}

	var result ToolSetGeneration
	err = s.sq.
		Update(ToolSetGenerationsTable).
		Set("state", toolsetschema.ToolSetReleaseStatePrimary).
		Set("updated_at", now).
		Where(sq.Eq{
			"tool_set_id": id,
			"generation":  generation,
			"state":       toolsetschema.ToolSetReleaseStateDraft,
			"revision":    expectedRevision,
			"deleted_at":  nil,
		}).
		Suffix("RETURNING " + strings.Join(result.cols(), ", ")).
		RunWith(tx).
		QueryRowContext(ctx).
		Scan(result.fields()...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrToolSetGenerationRevisionConflict
	}
	if err != nil {
		return nil, fmt.Errorf("failed to publish toolset generation: %w", err)
	}
	
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit toolset publication: %w", err)
	}
	return &result, nil
}
