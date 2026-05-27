package api_key

import (
	"context"
	"encoding/json"

	apauthcore "github.com/rmorlok/authproxy/internal/apauth/core"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httperr"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
)

// PersistCredentials extracts the api-key credential field values from
// credData (validated by the schema-step's JSON Schema upstream), encrypts
// the resulting plaintext as a single JSON blob, and inserts a fresh row
// into api_key_credentials. InsertApiKeyCredential soft-deletes any active
// row in the same transaction, so this is the rotation path as well as the
// initial-set path.
//
// User-visible error conditions surface as *httperr.Err (BadRequest); core's
// dispatcher returns them as-is to the HTTP layer.
func (f *factory) PersistCredentials(
	ctx context.Context,
	connection coreIface.Connection,
	placement *cschema.ApiKeyPlacement,
	credData map[string]any,
) error {
	if placement == nil {
		return httperr.InternalServerErrorMsg("api-key connector missing placement at credential submission time")
	}

	plaintext := database.ApiKeyCredentialPlaintext{}
	if v, ok := credData["api_key"].(string); ok {
		plaintext.ApiKey = v
	}
	if plaintext.ApiKey == "" {
		return httperr.BadRequest("api_key is required")
	}
	if placement.Type == cschema.ApiKeyPlacementBasic {
		if placement.UsernameField == "" {
			return httperr.InternalServerErrorMsg("basic placement missing username_field at credential submission time")
		}
		v, _ := credData[placement.UsernameField].(string)
		if v == "" {
			return httperr.BadRequestf("%q is required for basic placement", placement.UsernameField)
		}
		plaintext.Username = v
	}

	blobJSON, err := json.Marshal(plaintext)
	if err != nil {
		return httperr.InternalServerError(httperr.WithInternalErrorf("failed to marshal api-key plaintext: %w", err))
	}
	encrypted, err := f.encrypt.EncryptStringForNamespace(ctx, connection.GetNamespace(), string(blobJSON))
	if err != nil {
		return httperr.InternalServerError(httperr.WithInternalErrorf("failed to encrypt api-key credentials: %w", err))
	}

	actorId := apauthcore.GetAuthFromContext(ctx).MustGetActor().GetId()
	if _, err := f.db.InsertApiKeyCredential(ctx, connection.GetId(), encrypted, placement, &actorId); err != nil {
		return httperr.InternalServerError(httperr.WithInternalErrorf("failed to persist api-key credentials: %w", err))
	}
	return nil
}
