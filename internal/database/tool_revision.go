package database

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"math"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/util"
)

// ToolRevisionsTable retains immutable executable definitions for each Tool.
const ToolRevisionsTable = "tool_revisions"

// ToolRevision captures a prepared definition and its connection at publication.
// EncryptedDefinition contains the complete executable definition, including its
// schemas and aliases; callers own compilation, serialization, and encryption.
// ConnectionId is historical identity, so purging a former connection after a
// retarget must not delete this revision. Revisions are never updated in place,
// except that key rotation may replace ciphertext without changing the digest.
// Source provenance and retention policies will be added with imported Tools.
type ToolRevision struct {
	ToolId              apid.ID
	Revision            uint64
	ConnectionId        apid.ID
	EncryptedDefinition encfield.EncryptedField

	// DefinitionDigest is lowercase SHA-256 of the caller's canonical plaintext
	// definition, independent of randomized encryption and later key rotation.
	DefinitionDigest string
	CreatedAt        time.Time
	EncryptedAt      *time.Time
	DeletedAt        *time.Time
}

func init() {
	RegisterEncryptedField(EncryptedFieldRegistration{
		Table:            ToolRevisionsTable,
		PrimaryKeyCols:   []string{"tool_id", "revision"},
		EncryptedCols:    []string{"encrypted_definition"},
		JoinTable:        ToolsTable,
		JoinLocalCol:     "tool_id",
		JoinRemoteCol:    "id",
		JoinNamespaceCol: "namespace",
	})
}

// cols lists revision columns in migration order, matching fields and values.
func (r *ToolRevision) cols() []string {
	return []string{
		"tool_id",
		"revision",
		"connection_id",
		"encrypted_definition",
		"definition_digest",
		"created_at",
		"encrypted_at",
		"deleted_at",
	}
}

// fields supplies destinations for one immutable definition snapshot.
func (r *ToolRevision) fields() []any {
	return []any{
		&r.ToolId,
		&r.Revision,
		&r.ConnectionId,
		&r.EncryptedDefinition,
		&r.DefinitionDigest,
		&r.CreatedAt,
		&r.EncryptedAt,
		&r.DeletedAt,
	}
}

// values supplies revision insertion values using the encrypted-field codec.
func (r *ToolRevision) values() []any {
	return []any{
		r.ToolId,
		r.Revision,
		r.ConnectionId,
		r.EncryptedDefinition,
		r.DefinitionDigest,
		r.CreatedAt,
		r.EncryptedAt,
		r.DeletedAt,
	}
}

// Validate checks identities, SQL counter bounds, ciphertext presence, and digest
// shape. The database cannot verify plaintext or its digest without decryption.
func (r *ToolRevision) Validate() error {
	if r == nil {
		return errors.New("tool revision is required")
	}
	if err := validateToolID(r.ToolId); err != nil {
		return err
	}
	if err := validateToolRevision(r.Revision); err != nil {
		return err
	}
	if err := validateToolConnectionID(r.ConnectionId); err != nil {
		return err
	}
	if r.EncryptedDefinition.IsZero() {
		return errors.New("encrypted tool definition is required")
	}
	if len(r.DefinitionDigest) != 64 || strings.ToLower(r.DefinitionDigest) != r.DefinitionDigest {
		return errors.New("tool definition digest must be lowercase SHA-256 hex")
	}
	if _, err := hex.DecodeString(r.DefinitionDigest); err != nil {
		return errors.New("tool definition digest must be lowercase SHA-256 hex")
	}
	return nil
}

// validateToolRevision checks the positive integer range shared by both stores.
func validateToolRevision(revision uint64) error {
	if revision == 0 || revision > math.MaxInt64 {
		return errors.New("tool revision must be a positive signed 64-bit integer")
	}
	return nil
}

// newToolRevision captures the candidate's active identity and injected clock.
func newToolRevision(ctx context.Context, tool *Tool, definition encfield.EncryptedField, digest string) *ToolRevision {
	now := apctx.GetClock(ctx).Now()
	return &ToolRevision{
		ToolId:              tool.Id,
		Revision:            tool.ActiveRevision,
		ConnectionId:        tool.ConnectionId,
		EncryptedDefinition: definition,
		DefinitionDigest:    digest,
		CreatedAt:           now,
		EncryptedAt:         &now,
	}
}

// insertToolRevision appends a snapshot inside the pointer owner's transaction.
// An existing composite identity, including a tombstone, cannot be overwritten.
func (s *service) insertToolRevision(ctx context.Context, tx *sql.Tx, revision *ToolRevision) error {
	_, err := s.sq.Insert(ToolRevisionsTable).
		Columns(revision.cols()...).
		Values(revision.values()...).
		RunWith(tx).
		ExecContext(ctx)
	if err != nil {
		return wrapDatabaseMutationError("failed to append tool revision", err)
	}
	return nil
}

// getToolRevision reads either a historical definition or the active definition
// in one SQL snapshot. Only active reads require a live current connection.
func (s *service) getToolRevision(ctx context.Context, id apid.ID, revision uint64, current bool) (*ToolRevision, error) {
	var result ToolRevision

	columns := util.PrependStringToAll("r.", result.cols())
	query := s.sq.
		Select(columns...).
		From(ToolRevisionsTable + " r").
		Join(ToolsTable + " t ON t.id = r.tool_id").
		Join(NamespacesTable + " n ON n.path = t.namespace").
		Where(sq.Eq{
			"t.id":         id,
			"t.deleted_at": nil,
			"r.deleted_at": nil,
			"n.deleted_at": nil,
		})
	if current {
		query = query.Where("r.revision = t.active_revision").
			Join(ConnectionsTable + " c ON c.id = t.connection_id AND c.id = r.connection_id").
			Where(sq.Eq{"c.deleted_at": nil})
	} else {
		query = query.Where(sq.Eq{"r.revision": revision})
	}

	err := query.RunWith(s.db).
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

// GetCurrentToolRevision returns the active encrypted definition and its captured
// connection through a single joined read. This is a storage snapshot, not an
// authorization decision or execution-readiness check.
func (s *service) GetCurrentToolRevision(ctx context.Context, id apid.ID) (*ToolRevision, error) {
	if err := validateToolID(id); err != nil {
		return nil, err
	}
	return s.getToolRevision(ctx, id, 0, true)
}

// GetToolRevision reads retained history for inspection under a live Tool and
// namespace. The captured connection need not still exist. Historical revisions
// are not independently invokable; admission must select the current revision.
func (s *service) GetToolRevision(ctx context.Context, id apid.ID, revision uint64) (*ToolRevision, error) {
	if err := validateToolID(id); err != nil {
		return nil, err
	}
	if err := validateToolRevision(revision); err != nil {
		return nil, err
	}
	return s.getToolRevision(ctx, id, revision, false)
}
