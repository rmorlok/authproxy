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
// the callback transition. #352 (client_credentials grant) will introduce a
// second id for the synchronous token-exchange path.
const OAuth2AuthorizeStepId = "apxy:auth:oauth2_authorize"

// ManifestSetupSteps returns the OAuth2-emitted setup steps for this
// connection. The authorization_code grant (today's only supported grant)
// emits a single redirect step whose RenderRedirect resolves to the
// freshly-minted authorize URL (state + PKCE already persisted to redis).
func (f *factory) ManifestSetupSteps(connection coreIface.Connection, connector *cschema.Connector) []coreIface.ManifestSetupStep {
	if connector == nil || connector.Auth == nil {
		return nil
	}
	if _, ok := connector.Auth.Inner().(*cschema.AuthOAuth2); !ok {
		return nil
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
