package api_key

import (
	"context"
	"encoding/json"

	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/httperr"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
)

// ManifestSetupSteps returns the single form step api-key contributes to a
// connection's setup flow: a credential-collection form synthesized from the
// connector's placement. The OnSubmit closure validates the submitted form
// data against the synthesized JSON Schema and persists the resulting
// credential via the factory.
//
// Returns nil if the connector is not an api-key connector (defensive — the
// registry resolution should already guarantee this) or if the placement is
// missing.
func (f *factory) ManifestSetupSteps(connection coreIface.Connection, connector *cschema.Connector) []coreIface.ManifestSetupStep {
	if connector == nil || connector.Auth == nil {
		return nil
	}
	ak, ok := connector.Auth.Inner().(*cschema.AuthApiKey)
	if !ok {
		return nil
	}
	spec := synthesizeCredentialsStep(ak.Placement)
	if spec == nil {
		return nil
	}
	placement := ak.Placement
	return []coreIface.ManifestSetupStep{
		coreIface.NewFormStep(coreIface.FormStepConfig{
			Id:          spec.Id,
			Title:       spec.Title,
			Description: spec.Description,
			JsonSchema:  json.RawMessage(spec.JsonSchema),
			UiSchema:    json.RawMessage(spec.UiSchema),
			OnSubmit: func(ctx context.Context, data json.RawMessage) error {
				// Validate the submitted data against the synthesized schema.
				// ValidateAndMergeData with nil config returns a fresh map of
				// the validated fields; we hand it to PersistCredentials and
				// discard the map (the plaintext is then encrypted into the
				// api_key_credentials row).
				credData, err := spec.ValidateAndMergeData(spec.Id, data, nil)
				if err != nil {
					return httperr.BadRequest(err.Error())
				}
				return f.PersistCredentials(ctx, connection, placement, credData)
			},
		}),
	}
}
