package oauth2

import (
	"context"
	"encoding/json"
	"fmt"

	apauthcore "github.com/rmorlok/authproxy/internal/apauth/core"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/httperr"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
)

// OAuth2AuthorizeStepId is the manifest id for OAuth2's authorize-redirect
// step. Constant so core's dispatcher can compare against it when handling
// the callback transition.
const OAuth2AuthorizeStepId = "apxy:auth:oauth2_authorize"

// OAuth2ClientCredentialsStepId is the manifest id for OAuth2's synthesized
// client-credentials collection form.
const OAuth2ClientCredentialsStepId = "apxy:auth:oauth2_client_credentials"

// ManifestSetupSteps returns the OAuth2-emitted setup steps for this
// connection. authorization_code emits a redirect step; client_credentials
// emits a credential form that stores the submitted client id / secret and
// performs the token endpoint exchange on submit.
func (f *factory) ManifestSetupSteps(connection coreIface.Connection, connector *cschema.Connector) []coreIface.ManifestSetupStep {
	if connector == nil || connector.Auth == nil {
		return nil
	}
	auth, ok := connector.Auth.Inner().(*cschema.AuthOAuth2)
	if !ok {
		return nil
	}
	if auth.GetGrantTypeOrDefault() == cschema.OAuth2GrantClientCredentials {
		spec := synthesizeClientCredentialsStep(auth.GetTokenEndpointAuthMethodOrDefault())
		return []coreIface.ManifestSetupStep{
			coreIface.NewFormStep(coreIface.FormStepConfig{
				Id:          spec.Id,
				Title:       spec.Title,
				Description: spec.Description,
				JsonSchema:  json.RawMessage(spec.JsonSchema),
				UiSchema:    json.RawMessage(spec.UiSchema),
				OnSubmit: func(ctx context.Context, data json.RawMessage) error {
					credData, err := spec.ValidateAndMergeData(spec.Id, data, nil)
					if err != nil {
						return httperr.BadRequest(err.Error())
					}
					if err := f.PersistClientCredentials(ctx, connection, auth, credData); err != nil {
						return err
					}
					o2 := f.NewOAuth2(connection)
					return o2.ExchangeClientCredentials(ctx)
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
