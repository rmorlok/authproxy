package core

import (
	"context"
	"fmt"

	authcore "github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	toolschema "github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/util"
)

// authorizedToolInvocation holds the captured inputs that passed the caller's
// Tool permissions and optional revision precondition. It is private execution
// data, not a descriptor or API response. Authorization does not establish
// connection readiness or compile the Tool for execution.
type authorizedToolInvocation struct {
	// snapshot pins the revision, connection configuration, connector generation,
	// and source identity. Later admission steps must use these captured values
	// rather than rereading mutable records after authorization.
	snapshot *database.ToolExecutionSnapshot

	// definition is decoded from snapshot.Revision.EncryptedDefinition. Both
	// permission checks and eventual execution use this same definition: a new
	// revision cannot substitute different aliases or code after authorization.
	definition toolschema.ToolDefinition
}

// authorizeToolInvocation captures a live Tool and authorizes at least one of
// its declared verbs on its connection. Each verb must pass both the actor's
// permissions and any request-level restrictions; proxy permission is not
// required. Missing or unauthorized Tools return ErrNotFound, and only an
// authorized caller can receive ErrToolRevisionChanged for expectedRevision.
//
// The helper decrypts only the captured Tool definition. It does not hydrate a
// connection, resolve credentials, evaluate schemas, or perform upstream I/O.
// The remaining admission and execution steps must consume the returned capture.
func (s *service) authorizeToolInvocation(
	ctx context.Context,
	ra *authcore.RequestAuth,
	id apid.ID,
	expectedRevision *uint64,
) (*authorizedToolInvocation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if ra == nil || !ra.IsAuthenticated() {
		return nil, ErrNotFound
	}
	if id.IsNil() || id.ValidatePrefix(apid.PrefixTool) != nil {
		return nil, fmt.Errorf("%w: a tool id is required", ErrInvalidArgument)
	}
	if expectedRevision != nil && *expectedRevision == 0 {
		return nil, fmt.Errorf("%w: expectedRevision must be positive", ErrInvalidArgument)
	}

	snapshot, err := s.db.GetToolExecutionSnapshot(ctx, id)
	if err != nil {
		return nil, mapDatabaseError(err)
	}

	// Stop before decryption if cancellation arrived during the database read.
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	plaintext, err := s.encrypt.Decrypt(ctx, snapshot.Revision.EncryptedDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt tool %s revision %d: %w", id, snapshot.Revision.Revision, err)
	}

	var definition toolschema.ToolDefinition
	if err := util.DecodeJSONStrict(plaintext, &definition); err != nil {
		return nil, fmt.Errorf("failed to decode tool %s revision %d: %w", id, snapshot.Revision.Revision, err)
	}

	// Do not grant authorization if cancellation arrived during decryption or
	// decoding, including when a dependency completed without returning an error.
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	for _, verb := range definition.Verbs {
		// Allows intersects actor and JWT permissions for this exact alias. Two
		// different aliases separately granted by each cannot authorize a call.
		if !ra.Allows(snapshot.Connection.Namespace, "connections", verb, snapshot.Connection.Id.String()) {
			continue
		}

		// Check the revision only after authorization so a denied caller cannot
		// distinguish a stale descriptor from an unknown Tool ID.
		if expectedRevision != nil && *expectedRevision != snapshot.Revision.Revision {
			return nil, ErrToolRevisionChanged
		}
		return &authorizedToolInvocation{
			snapshot:   snapshot,
			definition: definition,
		}, nil
	}

	return nil, ErrNotFound
}
