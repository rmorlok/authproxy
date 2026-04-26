package iface

import (
	"context"
	"net/http"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apjs"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httpf"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
)

type Connection interface {
	/*
	 * Core fields
	 */

	GetId() apid.ID
	GetNamespace() string
	GetState() database.ConnectionState
	GetConnectorId() apid.ID
	GetConnectorVersion() uint64
	GetCreatedAt() time.Time
	GetUpdatedAt() time.Time
	GetDeletedAt() *time.Time
	GetLabels() map[string]string
	GetAnnotations() map[string]string
	GetSetupStep() *cschema.SetupStep
	GetSetupError() *string

	/*
	 * Nested entities
	 */

	GetConnectorVersionEntity() ConnectorVersion

	/*
	 * Methods
	 */

	SetState(ctx context.Context, state database.ConnectionState) error
	SetSetupStep(ctx context.Context, setupStep *cschema.SetupStep) error
	SetSetupError(ctx context.Context, setupError *string) error
	GetConfiguration(ctx context.Context) (map[string]any, error)
	SetConfiguration(ctx context.Context, data map[string]any) error
	GetMustacheContext(ctx context.Context) (map[string]any, error)
	GetProbe(probeId string) (Probe, error)
	GetProbes() []Probe
	ProxyRequest(
		ctx context.Context,
		reqType httpf.RequestType,
		req *ProxyRequest,
	) (*ProxyResponse, error)
	ProxyRequestRaw(
		ctx context.Context,
		reqType httpf.RequestType,
		req *ProxyRequest,
		w http.ResponseWriter,
	) error
	SubmitForm(ctx context.Context, req SubmitConnectionRequest) (InitiateConnectionResponse, error)
	GetCurrentSetupStepResponse(ctx context.Context) (InitiateConnectionResponse, error)
	GetDataSource(ctx context.Context, sourceId string) ([]apjs.DataSourceOption, error)
	Reconfigure(ctx context.Context) (InitiateConnectionResponse, error)

	// CancelSetup abandons an in-flight reconfigure on a ready connection by clearing
	// its setup_step and setup_error. The connection remains ready and its previously
	// stored configuration continues to apply.
	CancelSetup(ctx context.Context) error

	// HandleCredentialsEstablished advances the connection to the next setup phase after an
	// auth method has stored valid credentials. It transitions the connection to verify (and
	// enqueues probes) when the connector has probes, otherwise to configure:0 when the
	// connector has configure steps, otherwise it clears the setup step so the connection is
	// considered ready. Auth methods invoke this so post-auth state transitions stay
	// independent of the credential exchange mechanism.
	HandleCredentialsEstablished(ctx context.Context) (PostAuthOutcome, error)

	// HandleAuthFailed records a failure during the auth phase (e.g. an OAuth token exchange
	// error). It populates setup_error and moves setup_step to the auth_failed terminal
	// pseudo-step so the user is left in a retryable state — the marketplace UI surfaces the
	// error and offers retry/cancel via the connection retry endpoint.
	HandleAuthFailed(ctx context.Context, authErr error) error
}

// PostAuthOutcome describes what happened after credentials were established. SetupPending
// is true when the connection still has a setup step to complete (verify or configure);
// auth methods use this to decide whether the user should be sent to a "setup pending" URL
// or directly to the original return URL.
type PostAuthOutcome struct {
	SetupPending bool
}
