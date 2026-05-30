package oauth2

import (
	"context"
	"fmt"

	apauthcore "github.com/rmorlok/authproxy/internal/apauth/core"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
)

// OAuth2AuthorizeStepId is the manifest id for OAuth2's authorize-redirect
// step. Constant so core's dispatcher can compare against it when handling
// the callback transition.
const OAuth2AuthorizeStepId = "apxy:auth:oauth2_authorize"

// OAuth2ClientCredentialsStepId is the manifest id for OAuth2's synchronous
// client-credentials exchange step. Core recognizes this id as an immediate
// auth step.
const OAuth2ClientCredentialsStepId = "apxy:auth:oauth2_client_credentials"

// ManifestSetupSteps returns the OAuth2-emitted setup steps for this
// connection. authorization_code emits a redirect step; client_credentials
// emits an immediate step that core executes synchronously.
func (f *factory) ManifestSetupSteps(connection coreIface.Connection, connector *cschema.Connector) []coreIface.ManifestSetupStep {
	if connector == nil || connector.Auth == nil {
		return nil
	}
	auth, ok := connector.Auth.Inner().(*cschema.AuthOAuth2)
	if !ok {
		return nil
	}
	if auth.GetGrantTypeOrDefault() == cschema.OAuth2GrantClientCredentials {
		return []coreIface.ManifestSetupStep{
			coreIface.NewImmediateStep(coreIface.ImmediateStepConfig{
				Id:          OAuth2ClientCredentialsStepId,
				Title:       "Authorize",
				Description: "Authorizing this connection.",
				OnEnter: func(ctx context.Context) error {
					o2 := f.NewOAuth2(connection)
					if err := o2.ExchangeClientCredentials(ctx); err != nil {
						if recordErr := connection.HandleAuthFailed(ctx, err); recordErr != nil {
							return fmt.Errorf("failed to record auth failure (%v) after: %w", recordErr, err)
						}
					}
					return nil
				},
			}),
		}
	}
	return []coreIface.ManifestSetupStep{
		coreIface.NewRedirectStep(coreIface.RedirectStepConfig{
			Id:          OAuth2AuthorizeStepId,
			Title:       "Authorize",
			Description: "Sign in to authorize this connection.",
			Render: func(ctx context.Context, opts coreIface.RenderRedirectOptions) (coreIface.RedirectInfo, error) {
				if opts.ReturnToUrl == "" {
					return coreIface.RedirectInfo{}, fmt.Errorf("return_to_url is required for OAuth2 authorize step")
				}
				ra := apauthcore.GetAuthFromContext(ctx)
				o2 := f.NewOAuth2(connection)
				url, err := o2.SetStateAndGeneratePublicUrl(ctx, ra.MustGetActor(), opts.ReturnToUrl)
				if err != nil {
					return coreIface.RedirectInfo{}, err
				}
				return coreIface.RedirectInfo{URL: url}, nil
			},
		}),
	}
}
