package oauth2

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/rmorlok/authproxy/internal/aplog"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

// scopeMismatchOutcome captures the differences between the scopes the connector definition
// requested and the scopes the provider actually granted. Required scopes that the provider
// declined to grant are surfaced as errors so the connection lands in auth_failed; optional
// scopes are logged so callers can correlate against the /scopes endpoint.
type scopeMismatchOutcome struct {
	missingRequired []string
	missingOptional []string
	extraGranted    []string
}

// detectScopeMismatch compares the scopes declared on the connector against the scopes echoed
// by the provider. The granted set falls back to the requested set when the provider omits the
// `scope` parameter (RFC 6749 §5.1 — silent agreement to the request).
//
// The declared scopes are expected to have had predicates resolved prior to calling this function.
func detectScopeMismatch(declaredEffectiveScopes []sconfig.Scope, granted string) (scopeMismatchOutcome, error) {
	grantedSet := scopeSet(granted)

	var outcome scopeMismatchOutcome
	declaredIds := make(map[string]struct{}, len(declaredEffectiveScopes))
	for _, s := range declaredEffectiveScopes {
		declaredIds[s.Id] = struct{}{}
		if _, ok := grantedSet[s.Id]; ok {
			continue
		}

		// We can use nil here because these are effective scopes that have
		// had predicates resolved.
		required, err := s.IsRequired(nil)
		if err != nil {
			return scopeMismatchOutcome{}, fmt.Errorf("scope %q required: %w", s.Id, err)
		}
		if required {
			outcome.missingRequired = append(outcome.missingRequired, s.Id)
		} else {
			outcome.missingOptional = append(outcome.missingOptional, s.Id)
		}
	}
	for g := range grantedSet {
		if _, ok := declaredIds[g]; !ok {
			outcome.extraGranted = append(outcome.extraGranted, g)
		}
	}
	sort.Strings(outcome.missingRequired)
	sort.Strings(outcome.missingOptional)
	sort.Strings(outcome.extraGranted)
	return outcome, nil
}

// applyScopeMismatch records the outcome on the connection. Required-scope misses produce an
// error so the caller can route through HandleAuthFailed; optional misses and extra grants are
// logged but never block — callers inspect the /scopes endpoint to see the divergence.
func (o *oAuth2Connection) applyScopeMismatch(ctx context.Context, outcome scopeMismatchOutcome) error {
	baseLogger := o.logger
	if baseLogger == nil {
		baseLogger = slog.Default()
	}
	logger := aplog.NewBuilder(baseLogger).
		WithCtx(ctx).
		WithConnectionId(o.connection.GetId()).
		Build()

	if len(outcome.extraGranted) > 0 {
		logger.Info("provider granted extra oauth2 scopes beyond those declared",
			"extra_scopes", strings.Join(outcome.extraGranted, " "))
	}

	if len(outcome.missingOptional) > 0 {
		logger.Warn("provider did not grant some optional oauth2 scopes",
			"missing_optional_scopes", strings.Join(outcome.missingOptional, " "))
	}

	if len(outcome.missingRequired) > 0 {
		return fmt.Errorf("required oauth2 scopes were not granted by provider: %s",
			strings.Join(outcome.missingRequired, " "))
	}

	return nil
}
