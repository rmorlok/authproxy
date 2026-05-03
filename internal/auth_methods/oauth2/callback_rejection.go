package oauth2

import (
	"context"
	"errors"
	"log/slog"

	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/apid"
)

// rejectionCategory identifies the class of failure that caused us to refuse
// an OAuth2 callback. The category is the primary observability hook for
// security operators investigating CSRF, replay, or cross-tenant probing —
// alerts and dashboards key off these stable string values, so existing
// values must not be renamed without updating downstream consumers.
type rejectionCategory string

const (
	// rejectionMissingState — the callback URL had no `state` query
	// parameter. Almost always a malformed request or a probe.
	rejectionMissingState rejectionCategory = "missing_state"
	// rejectionInvalidStateFormat — `state` was present but did not parse
	// as an apid.ID. The value never reached Redis.
	rejectionInvalidStateFormat rejectionCategory = "invalid_state_format"
	// rejectionUnknownState — the parsed state id was not present in Redis.
	// Covers both genuinely unknown ids and replays of states already
	// consumed (we delete on success).
	rejectionUnknownState rejectionCategory = "unknown_state"
	// rejectionTamperedState — the Redis payload failed AEAD authentication
	// on decrypt. The state envelope was modified outside this proxy.
	rejectionTamperedState rejectionCategory = "tampered_state"
	// rejectionStateLoadError — Redis or envelope parsing failed for a
	// reason other than not-found or AEAD failure (e.g., transport error,
	// malformed JSON after decrypt). Treated as a rejection because we
	// cannot trust a state we cannot fully validate.
	rejectionStateLoadError rejectionCategory = "state_load_error"
	// rejectionStateInvalid — the decrypted state was missing required
	// fields per state.IsValid (empty namespace, zero ids, etc.).
	rejectionStateInvalid rejectionCategory = "state_invalid"
	// rejectionExpiredState — the state's ExpiresAt is in the past.
	rejectionExpiredState rejectionCategory = "expired_state"
	// rejectionActorMismatch — the inbound actor's id does not match the
	// actor id recorded on the state. A different user is attempting to
	// complete someone else's flow.
	rejectionActorMismatch rejectionCategory = "actor_mismatch"
	// rejectionNamespaceMismatchActor — the inbound actor's namespace does
	// not match the state's namespace. Cross-tenant probe via a stolen or
	// guessed state id.
	rejectionNamespaceMismatchActor rejectionCategory = "namespace_mismatch_actor"
	// rejectionNamespaceMismatchConnection — the connection referenced by
	// the state lives in a different namespace than the state itself.
	// State was crafted against another tenant's connection id.
	rejectionNamespaceMismatchConnection rejectionCategory = "namespace_mismatch_connection"
	// rejectionConnectionNotFound — the connection id on the state has no
	// matching record. Usually a deleted connection or a fabricated id.
	rejectionConnectionNotFound rejectionCategory = "connection_not_found"
	// rejectionConnectorTypeMismatch — the connection's connector is not
	// configured for OAuth2 auth. The state was crafted against a
	// non-OAuth2 connection.
	rejectionConnectorTypeMismatch rejectionCategory = "connector_type_mismatch"
)

// rejectionAttrs carries the safe-to-log identifiers associated with a
// rejected callback. All fields are optional; only the populated ones are
// emitted, so callers can supply just what they have at the moment of
// failure. apid.IDs are UUID-prefixed and opaque, so they're safe to log.
// Never put secrets, tokens, or raw query strings here.
type rejectionAttrs struct {
	StateId      apid.ID
	ActorId      apid.ID
	ConnectionId apid.ID
	Namespace    string
	Err          error
}

// rejectionEventMessage is the single message string for every callback
// rejection. Operators filter on this exact string when correlating events
// across categories — keep it stable.
const rejectionEventMessage = "oauth callback rejected"

// emitCallbackRejection logs a structured "oauth callback rejected" event
// at Warn. The category is the primary axis for downstream alerting; the
// other attrs are populated only when the caller has them in hand. This
// helper is the single entrypoint for rejection events so a future change
// (sampling, metric emission, sink change) lives in one place.
func emitCallbackRejection(ctx context.Context, logger *slog.Logger, category rejectionCategory, attrs rejectionAttrs) {
	if logger == nil {
		logger = slog.Default()
	}

	b := aplog.NewBuilder(logger).WithCtx(ctx)
	if attrs.ConnectionId != apid.Nil {
		b = b.WithConnectionId(attrs.ConnectionId)
	}
	if attrs.Namespace != "" {
		b = b.WithNamespace(attrs.Namespace)
	}

	args := []any{slog.String("category", string(category))}
	if attrs.StateId != apid.Nil {
		args = append(args, slog.String("state_id", attrs.StateId.String()))
	}
	if attrs.ActorId != apid.Nil {
		args = append(args, slog.String("actor_id", attrs.ActorId.String()))
	}
	if attrs.Err != nil {
		args = append(args, slog.String("error", attrs.Err.Error()))
	}

	b.Build().WarnContext(ctx, rejectionEventMessage, args...)
}

// errStateNotFound and errStateTampered let callers of readStateFromRedis
// distinguish "no such state" from "state was modified outside the proxy"
// without inspecting error strings. Keeping these as sentinels (rather
// than pushing rejection emission into readStateFromRedis itself) lets us
// emit at the call site where ActorId is also in scope.
var (
	errStateNotFound = errors.New("oauth state not found")
	errStateTampered = errors.New("oauth state failed integrity check")
)

// EmitMissingStateRejection is invoked by the public oauth2 callback route
// when the inbound request has no `state` query parameter. The state id
// hasn't been parsed yet, so attrs carries no identifiers.
func EmitMissingStateRejection(ctx context.Context, logger *slog.Logger, err error) {
	emitCallbackRejection(ctx, logger, rejectionMissingState, rejectionAttrs{Err: err})
}

// EmitInvalidStateFormatRejection is invoked by the public oauth2 callback
// route when the `state` query parameter fails to parse as an apid.ID.
// The raw value isn't logged because it's attacker-controlled.
func EmitInvalidStateFormatRejection(ctx context.Context, logger *slog.Logger, err error) {
	emitCallbackRejection(ctx, logger, rejectionInvalidStateFormat, rejectionAttrs{Err: err})
}
