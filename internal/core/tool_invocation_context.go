package core

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/tools/render"
	"github.com/rmorlok/authproxy/internal/util"
)

// errToolConnectionUnavailable means the authorized connection's captured
// lifecycle state prevents execution. The caller retains the authorized capture
// to form a CONNECTION_UNAVAILABLE result with notStarted and matching identity.
var errToolConnectionUnavailable = errors.New("tool connection is not configured")

// preparedToolInvocation pairs the template context with the only HTTP client
// execution may use. Both derive from the same authorized connection capture.
// It is private preparation data, not an API response or a running invocation.
type preparedToolInvocation struct {
	// snapshot exposes configuration and metadata to templates, with Params left
	// unset for the executor to replace with validated caller arguments. Maps and
	// source pointers are detached from the client's private connection state.
	snapshot render.Input

	// client uses the captured connector/configuration and authorized actor.
	// Credentials and HTTP middleware remain lazy until its first Do; credentials
	// may refresh through the normal proxy lifecycle during execution.
	client iface.HTTPClient
}

// prepareToolInvocation checks known connection readiness and hydrates only the
// captured rows. It must follow authorizeToolInvocation: readiness and private
// configuration are never inspected for an unauthorized caller. The caller
// retains authorized for definition compilation and error/result identities.
//
// Unconfigured lifecycle states return errToolConnectionUnavailable; an
// unsupported connector auth method returns ErrProxyNotImplemented. Decryption,
// decoding, and cancellation failures remain internal Go errors. This step does
// not compile the Tool, evaluate schemas, load credentials, or send requests.
func (s *service) prepareToolInvocation(
	ctx context.Context,
	authorized *authorizedToolInvocation,
) (*preparedToolInvocation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if authorized == nil ||
		authorized.snapshot == nil ||
		authorized.actor == nil ||
		authorized.actor.Id.IsNil() {
		return nil, fmt.Errorf("%w: an authorized tool invocation is required", ErrInvalidArgument)
	}

	captured := authorized.snapshot
	if captured.Connection.State != database.ConnectionStateConfigured {
		return nil, errToolConnectionUnavailable
	}

	// Health is advisory, and a configured connection may be going through
	// reconfiguration or reauthentication. Neither a failed probe nor an active
	// setup step alone proves its current configuration/credentials unusable.
	connector := wrapConnector(captured.Connector, s)
	definition, err := connector.getDefinitionWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load tool connection's connector definition: %w", err)
	}

	if s.getAuthMethodFactory(definition) == nil {
		return nil, ErrProxyNotImplemented
	}

	// Use the pinned generation directly: getConnectionForDb would reread the
	// connector. Preloading its definition above also keeps the lazy client's
	// first Do from falling back to context.Background() for decryption.
	storedConnection := captured.Connection
	storedConnection.Labels = maps.Clone(storedConnection.Labels)
	storedConnection.Annotations = maps.Clone(storedConnection.Annotations)
	storedConnection.EncryptedConfiguration = util.CloneValue(storedConnection.EncryptedConfiguration)
	connection := wrapConnection(&storedConnection, connector, s)

	cfg, err := connection.GetConfiguration(ctx)
	if err != nil {
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	client, err := connection.HTTPClient(iface.HTTPClientOptions{
		Actor: iface.ActingAs(authorized.actor),
	})
	if err != nil {
		return nil, err
	}

	return &preparedToolInvocation{
		client: client,
		snapshot: render.Input{
			Cfg:                 cfg,
			Labels:              maps.Clone(map[string]string(storedConnection.Labels)),
			Annotations:         maps.Clone(map[string]string(storedConnection.Annotations)),
			ConnectionID:        storedConnection.Id.String(),
			ConnectionNamespace: storedConnection.Namespace,
			ToolID:              captured.Revision.ToolId.String(),
			Revision:            captured.Revision.Revision,
			ToolSetID:           captured.ToolSetId.String(),
			ToolSetGeneration:   util.CloneValue(captured.Revision.SourceGeneration),
		},
	}, nil
}
