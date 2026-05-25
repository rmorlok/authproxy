package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/encfield"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
)

func init() {
	RegisterEncryptedField(EncryptedFieldRegistration{
		Table:            ApiKeyCredentialsTable,
		PrimaryKeyCols:   []string{"id"},
		EncryptedCols:    []string{"encrypted_credentials"},
		JoinTable:        ConnectionsTable,
		JoinLocalCol:     "connection_id",
		JoinRemoteCol:    "id",
		JoinNamespaceCol: "namespace",
	})
}

const ApiKeyCredentialsTable = "api_key_credentials"

// ApiKeyCredentialPlaintext is the canonical plaintext shape stored, encrypted,
// inside ApiKeyCredential.EncryptedCredentials. Defined here so callers that
// encrypt (the connection-initiate submit handler) and callers that decrypt
// (the api-key proxy) share one contract. The database itself never inspects
// the plaintext — it only stores the encrypted blob.
type ApiKeyCredentialPlaintext struct {
	ApiKey   string `json:"api_key"`
	Username string `json:"username,omitempty"`
}

// ApiKeyCredential is one row in the api_key_credentials table — the encrypted
// credential blob (api key and, for basic placement, username) submitted by a
// user for a connection. The encrypted_credentials column stores a single
// opaque encrypted blob; the substructure inside (e.g. {"api_key": "...",
// "username": "..."}) is decided by the encrypt/decrypt layer that owns the
// plaintext shape — the database is agnostic to it.
//
// Rotation produces a new row and soft-deletes the prior, so the history of
// credentials is preserved. At most one row per connection has
// deleted_at IS NULL at any given moment.
type ApiKeyCredential struct {
	Id                   apid.ID
	ConnectionId         apid.ID                  // FK to Connection; not enforced by DB
	EncryptedCredentials encfield.EncryptedField  // Opaque encrypted blob (api key + optional username)
	PlacementSnapshot    *cschema.ApiKeyPlacement // Placement config at submission time
	CreatedByActorId     *apid.ID                 // Actor who submitted (or rotated to) this credential
	LastValidatedAt      *time.Time               // Most recent successful probe against this credential
	CreatedAt            time.Time
	EncryptedAt          *time.Time
	DeletedAt            *time.Time
}

func (c *ApiKeyCredential) cols() []string {
	return []string{
		"id",
		"connection_id",
		"encrypted_credentials",
		"placement_snapshot",
		"created_by_actor_id",
		"last_validated_at",
		"created_at",
		"encrypted_at",
		"deleted_at",
	}
}

func (c *ApiKeyCredential) fields() []any {
	return []any{
		&c.Id,
		&c.ConnectionId,
		&c.EncryptedCredentials,
		(*apiKeyPlacementDB)(c.placementPtr()),
		&c.CreatedByActorId,
		&c.LastValidatedAt,
		&c.CreatedAt,
		&c.EncryptedAt,
		&c.DeletedAt,
	}
}

func (c *ApiKeyCredential) values() []any {
	var placementVal driver.Value
	if c.PlacementSnapshot != nil {
		placementVal = apiKeyPlacementDB(*c.PlacementSnapshot)
	}
	return []any{
		c.Id,
		c.ConnectionId,
		c.EncryptedCredentials,
		placementVal,
		c.CreatedByActorId,
		c.LastValidatedAt,
		c.CreatedAt,
		c.EncryptedAt,
		c.DeletedAt,
	}
}

// placementPtr returns a non-nil pointer to PlacementSnapshot so that the Scan
// path can decode JSON into the (possibly newly allocated) struct.
func (c *ApiKeyCredential) placementPtr() *cschema.ApiKeyPlacement {
	if c.PlacementSnapshot == nil {
		c.PlacementSnapshot = &cschema.ApiKeyPlacement{}
	}
	return c.PlacementSnapshot
}

// apiKeyPlacementDB is the database envelope for ApiKeyPlacement — implements
// driver.Valuer + sql.Scanner for the placement_snapshot column. Stored as JSON
// (text in SQLite, jsonb in Postgres).
type apiKeyPlacementDB cschema.ApiKeyPlacement

func (p apiKeyPlacementDB) Value() (driver.Value, error) {
	return json.Marshal(cschema.ApiKeyPlacement(p))
}

func (p *apiKeyPlacementDB) Scan(src interface{}) error {
	var b []byte
	switch v := src.(type) {
	case nil:
		return nil
	case []byte:
		if len(v) == 0 {
			return nil
		}
		b = v
	case string:
		if v == "" {
			return nil
		}
		b = []byte(v)
	default:
		return fmt.Errorf("apiKeyPlacementDB: cannot scan %T", src)
	}
	return json.Unmarshal(b, (*cschema.ApiKeyPlacement)(p))
}

func (c *ApiKeyCredential) Validate() error {
	result := &multierror.Error{}

	if c.Id == apid.Nil {
		result = multierror.Append(result, errors.New("api key credential id is required"))
	} else if err := c.Id.ValidatePrefix(apid.PrefixApiKeyCredential); err != nil {
		result = multierror.Append(result, fmt.Errorf("invalid api key credential id: %w", err))
	}

	if c.ConnectionId == apid.Nil {
		result = multierror.Append(result, errors.New("api key credential connection id is required"))
	} else if err := c.ConnectionId.ValidatePrefix(apid.PrefixConnection); err != nil {
		result = multierror.Append(result, fmt.Errorf("invalid api key credential connection id: %w", err))
	}

	if c.CreatedByActorId != nil {
		if err := c.CreatedByActorId.ValidatePrefix(apid.PrefixActor); err != nil {
			result = multierror.Append(result, fmt.Errorf("invalid api key credential created_by_actor_id: %w", err))
		}
	}

	return result.ErrorOrNil()
}

// GetActiveApiKeyCredential returns the single non-deleted credential for the
// connection, or ErrNotFound if none exists. By the insert-and-soft-delete
// invariant there is at most one such row.
func (s *service) GetActiveApiKeyCredential(
	ctx context.Context,
	connectionId apid.ID,
) (*ApiKeyCredential, error) {
	var result ApiKeyCredential
	err := s.sq.
		Select(result.cols()...).
		From(ApiKeyCredentialsTable).
		Where(sq.Eq{
			"connection_id": connectionId,
			"deleted_at":    nil,
		}).
		OrderBy("created_at DESC").
		Limit(1).
		RunWith(s.db).
		QueryRow().
		Scan(result.fields()...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("no active api key credential for connection: %w", ErrNotFound)
		}
		return nil, err
	}
	return &result, nil
}

// InsertApiKeyCredential stores a new credential blob for the connection, soft-
// deleting any previously-active row in the same transaction so that exactly
// one credential is active per connection at all times.
//
// encryptedCredentials is the opaque encrypted blob produced by the auth
// layer; its plaintext substructure (e.g. api key + username for the basic
// placement) is decided by that layer and is not interpreted here. placement
// may be nil, though typically every insert carries a snapshot of the
// placement config in use at submission time.
func (s *service) InsertApiKeyCredential(
	ctx context.Context,
	connectionId apid.ID,
	encryptedCredentials encfield.EncryptedField,
	placement *cschema.ApiKeyPlacement,
	createdByActorId *apid.ID,
) (*ApiKeyCredential, error) {
	logger := aplog.NewBuilder(s.logger).
		WithCtx(ctx).
		WithConnectionId(connectionId).
		Build()
	logger.Debug("inserting new api key credential")

	now := apctx.GetClock(ctx).Now()
	var newCred *ApiKeyCredential

	err := s.transaction(func(tx *sql.Tx) error {
		dbResult, err := s.sq.Update(ApiKeyCredentialsTable).
			Set("deleted_at", now).
			Where(sq.Eq{
				"connection_id": connectionId,
				"deleted_at":    nil,
			}).
			RunWith(tx).
			Exec()
		if err != nil {
			return fmt.Errorf("failed to soft delete prior api key credentials: %w", err)
		}
		affected, err := dbResult.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to soft delete prior api key credentials: %w", err)
		}
		logger.Info("soft-deleted prior api key credentials for connection", "affected", affected)

		var placementCopy *cschema.ApiKeyPlacement
		if placement != nil {
			placementCopy = placement.Clone()
		}

		newCred = &ApiKeyCredential{
			Id:                   apctx.GetIdGenerator(ctx).New(apid.PrefixApiKeyCredential),
			ConnectionId:         connectionId,
			EncryptedCredentials: encryptedCredentials,
			PlacementSnapshot:    placementCopy,
			CreatedByActorId:     createdByActorId,
			CreatedAt:            now,
		}

		if err := newCred.Validate(); err != nil {
			return err
		}

		insertResult, err := s.sq.
			Insert(ApiKeyCredentialsTable).
			Columns(newCred.cols()...).
			Values(newCred.values()...).
			RunWith(tx).
			Exec()
		if err != nil {
			return fmt.Errorf("failed to create api key credential: %w", err)
		}
		inserted, err := insertResult.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to create api key credential: %w", err)
		}
		if inserted == 0 {
			return errors.New("failed to create api key credential; no rows inserted")
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return newCred, nil
}

// UpdateApiKeyCredentialLastValidated stamps the last_validated_at column on
// the credential. Called by probes on success against an api-key connection.
// Returns ErrNotFound if the credential row does not exist (e.g. it was
// rotated out before the probe outcome landed).
func (s *service) UpdateApiKeyCredentialLastValidated(
	ctx context.Context,
	credentialId apid.ID,
	at time.Time,
) error {
	dbResult, err := s.sq.
		Update(ApiKeyCredentialsTable).
		Set("last_validated_at", at).
		Where(sq.Eq{"id": credentialId}).
		RunWith(s.db).
		Exec()
	if err != nil {
		return fmt.Errorf("failed to update api key credential last_validated_at: %w", err)
	}
	affected, err := dbResult.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to update api key credential last_validated_at: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	if affected > 1 {
		return fmt.Errorf("multiple api key credentials updated for single id: %w", ErrViolation)
	}
	return nil
}

// DeleteAllApiKeyCredentialsForConnection soft-deletes every api-key credential
// row for the connection. Used when revoking a connection.
func (s *service) DeleteAllApiKeyCredentialsForConnection(
	ctx context.Context,
	connectionId apid.ID,
) error {
	logger := aplog.NewBuilder(s.logger).
		WithCtx(ctx).
		WithConnectionId(connectionId).
		Build()
	logger.Debug("deleting all api key credentials for connection")

	now := apctx.GetClock(ctx).Now()
	dbResult, err := s.sq.
		Update(ApiKeyCredentialsTable).
		Set("deleted_at", now).
		Where(sq.Eq{
			"connection_id": connectionId,
			"deleted_at":    nil,
		}).
		RunWith(s.db).
		Exec()
	if err != nil {
		return fmt.Errorf("failed to soft delete api key credentials for connection: %w", err)
	}
	affected, err := dbResult.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to soft delete api key credentials for connection: %w", err)
	}
	logger.Info("deleted api key credentials for connection", "affected", affected)
	return nil
}
