package core

import (
	"context"
	"fmt"
	"time"

	aphttp "github.com/rmorlok/authproxy/internal/apjs/http"
	"github.com/rmorlok/authproxy/internal/database"
	toolschema "github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/tools/javascript"
	"github.com/rmorlok/authproxy/internal/tools/jsexec"
	"github.com/rmorlok/authproxy/internal/tools/render"
	"github.com/rmorlok/authproxy/internal/tools/validation"
)

// toolPublishJavaScriptLimits is the fixed publication/installation profile,
// independent of tools.invocation. Initialization uses finite time, stack, and
// native work/allocation allowances; authored caps may lower them. The remaining
// fields let jsexec compile the complete definition and schemas, but publication
// never executes the program or supplies arguments or HTTP capabilities.
var toolPublishJavaScriptLimits = jsexec.Limits{
	Render: render.Limits{MaxBytes: 8 << 20, MaxDepth: 64, MaxSteps: 1_000_000},
	Validation: validation.Limits{
		MaxInputBytes: 1 << 20, MaxOutputBytes: 8 << 20,
		MaxDepth: 64, MaxValues: 1_000_000, MaxNumberDigits: 4096,
	},
	Runtime: javascript.Limits{
		// Native initialization helpers share one allowance, sized like the
		// pure response-transform publication profile's bytes and value nodes.
		MaxWorkUnits:       16*(8<<20+8<<20) + 32*1_000_000,
		MaxAllocationUnits: 16*(8<<20+8<<20) + 32*1_000_000,
		MaxSourceBytes:     256 << 10,
		MaxInputBytes:      16 << 20,
		MaxOutputBytes:     8 << 20,
		MaxDepth:           64,
		MaxNodes:           1_000_000,
		MaxStackDepth:      256,
		Timeout:            5 * time.Second,
	},
	HTTP: aphttp.Limits{
		Timeout: 5 * time.Second, MaxRequests: 20,
		MaxResponseBytes: 8 << 20, MaxStreamBytes: 8 << 20, MaxHeaderBytes: 64 << 10,
	},
	MaxRequestBytes:       1 << 20,
	MaxPendingOperations:  8,
	MaxEncodedResultBytes: 16 << 20,
}

// validateToolJavaScriptPublication checks a Tool with the helper library pinned
// by its resolved connection and returns the hydrated connector it validated.
// Standalone publication and ToolSet inventory preparation share this check.
// It loads only the connector
// definition, never connection configuration, credentials, or an HTTP client.
// Setup and disabled connections can therefore have Tools defined in advance.
//
// Initialization runs without arguments, context, authored logging, or I/O and
// does not invoke execute. Invalid authored code/schema wraps ErrInvalidArgument;
// dependency failures remain internal errors, and caller cancellation wins over
// validation failures. Preparation has its own fixed deadline; the authored
// timeout starts only when initialization begins. Synchronous JavaScript/schema
// compilation is checked after returning; schema compilation still needs the
// separately tracked work bound in #948.
func (s *service) validateToolJavaScriptPublication(
	ctx context.Context,
	definition *toolschema.ToolDefinition,
	connection *database.Connection,
) (*Connector, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := definition.Validate(nil); err != nil {
		return nil, fmt.Errorf("%w: spec: %w", ErrInvalidArgument, err)
	}
	limits, err := jsexec.ResolveLimits(toolPublishJavaScriptLimits, definition.Limits)
	if err != nil {
		return nil, fmt.Errorf("%w: spec.limits: %w", ErrInvalidArgument, err)
	}
	// Dependency latency must not consume the Tool's authored initialization
	// allowance or turn a slow database/decryption into invalid authored code.
	preparationCtx, cancel := context.WithTimeout(ctx, toolPublishJavaScriptLimits.Runtime.Timeout)
	defer cancel()

	// Preparation cancellation/timeouts are infrastructure errors, even when a
	// synchronous compiler returns an authored failure after the deadline ends.
	failure := func(err error, authored bool) error {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		if contextErr := preparationCtx.Err(); contextErr != nil {
			return contextErr
		}
		if err != nil && authored {
			return fmt.Errorf("%w: spec.javascript publication: %w", ErrInvalidArgument, err)
		}
		return err
	}

	row, err := s.db.GetConnectorGeneration(preparationCtx, connection.ConnectorId, connection.ConnectorGeneration)
	if err = failure(err, false); err != nil {
		return nil, err
	}
	connector := wrapConnector(*row, s)
	// Load separately so storage/decryption errors cannot be reported as bad
	// authored JavaScript. The bounded helper lookup below reuses this cache.
	_, err = connector.getDefinitionWithContext(preparationCtx)
	if err = failure(err, false); err != nil {
		return nil, err
	}
	library, err := connector.getBoundedJavascriptLibrary(preparationCtx, *definition.Javascript, limits.Runtime)
	if err = failure(err, true); err != nil {
		return nil, err
	}
	program, err := jsexec.Compile(*definition, library, limits)
	if err = failure(err, true); err != nil {
		return nil, err
	}
	cancel()

	// Validate starts its own authored-lowered runtime deadline. An initialization
	// timeout rejects the code without including time spent waiting for dependencies.
	err = program.Validate(ctx)
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	if err != nil {
		return nil, fmt.Errorf("%w: spec.javascript publication: %w", ErrInvalidArgument, err)
	}
	return connector, nil
}
