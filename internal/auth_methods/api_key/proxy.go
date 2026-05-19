package api_key

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/rmorlok/authproxy/internal/database"
)

// resolveAuth fetches the active credential row, decrypts the plaintext, and
// computes the authentication application for the placement that was in effect
// when the credential was submitted. Every credential row carries a placement
// snapshot — InsertApiKeyCredential always captures one — so a missing snapshot
// indicates corruption and surfaces an error rather than guessing.
//
// Each call loads the active credential fresh from the database so that a
// rotation (which inserts a new row and soft-deletes the prior) takes effect
// immediately on the next request — no in-memory cache to invalidate.
func (a *apiKeyConnection) resolveAuth(ctx context.Context) (authApplication, error) {
	cred, err := a.db.GetActiveApiKeyCredential(ctx, a.connection.GetId())
	if err != nil {
		return authApplication{}, fmt.Errorf("failed to load api-key credential: %w", err)
	}

	if cred.PlacementSnapshot == nil {
		return authApplication{}, errors.New("api-key credential is missing its placement snapshot")
	}

	decrypted, err := a.encrypt.DecryptString(ctx, cred.EncryptedCredentials)
	if err != nil {
		return authApplication{}, fmt.Errorf("failed to decrypt api-key credential: %w", err)
	}
	var plaintext database.ApiKeyCredentialPlaintext
	if err := json.Unmarshal([]byte(decrypted), &plaintext); err != nil {
		return authApplication{}, fmt.Errorf("failed to unmarshal api-key plaintext: %w", err)
	}

	return computeAuthApplication(cred.PlacementSnapshot, plaintext)
}
