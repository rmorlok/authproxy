package api_key

import (
	"encoding/base64"
	"testing"

	"github.com/rmorlok/authproxy/internal/database"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeAuthApplication_Bearer(t *testing.T) {
	app, err := computeAuthApplication(
		&cschema.ApiKeyPlacement{Type: cschema.ApiKeyPlacementBearer},
		database.ApiKeyCredentialPlaintext{ApiKey: "sk-abc-123"},
	)
	require.NoError(t, err)
	assert.Equal(t, "Authorization", app.HeaderName)
	assert.Equal(t, "Bearer sk-abc-123", app.HeaderValue)
	assert.Empty(t, app.QueryName)
}

func TestComputeAuthApplication_Header(t *testing.T) {
	t.Run("no prefix", func(t *testing.T) {
		app, err := computeAuthApplication(
			&cschema.ApiKeyPlacement{
				Type:       cschema.ApiKeyPlacementHeader,
				HeaderName: "X-API-Key",
			},
			database.ApiKeyCredentialPlaintext{ApiKey: "key-1"},
		)
		require.NoError(t, err)
		assert.Equal(t, "X-API-Key", app.HeaderName)
		assert.Equal(t, "key-1", app.HeaderValue)
	})

	t.Run("with prefix and trailing space", func(t *testing.T) {
		app, err := computeAuthApplication(
			&cschema.ApiKeyPlacement{
				Type:       cschema.ApiKeyPlacementHeader,
				HeaderName: "Authorization",
				Prefix:     "Token ",
			},
			database.ApiKeyCredentialPlaintext{ApiKey: "abc"},
		)
		require.NoError(t, err)
		assert.Equal(t, "Authorization", app.HeaderName)
		assert.Equal(t, "Token abc", app.HeaderValue)
	})

	t.Run("with prefix and no space", func(t *testing.T) {
		// Prefix is applied verbatim — connector author controls separator.
		app, err := computeAuthApplication(
			&cschema.ApiKeyPlacement{
				Type:       cschema.ApiKeyPlacementHeader,
				HeaderName: "X-Key",
				Prefix:     "v1_",
			},
			database.ApiKeyCredentialPlaintext{ApiKey: "abc"},
		)
		require.NoError(t, err)
		assert.Equal(t, "v1_abc", app.HeaderValue)
	})

	t.Run("missing header_name", func(t *testing.T) {
		_, err := computeAuthApplication(
			&cschema.ApiKeyPlacement{Type: cschema.ApiKeyPlacementHeader},
			database.ApiKeyCredentialPlaintext{ApiKey: "abc"},
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "header_name")
	})
}

func TestComputeAuthApplication_Query(t *testing.T) {
	app, err := computeAuthApplication(
		&cschema.ApiKeyPlacement{
			Type:      cschema.ApiKeyPlacementQuery,
			ParamName: "api_key",
		},
		database.ApiKeyCredentialPlaintext{ApiKey: "abc&xyz"},
	)
	require.NoError(t, err)
	assert.Empty(t, app.HeaderName)
	assert.Equal(t, "api_key", app.QueryName)
	assert.Equal(t, "abc&xyz", app.QueryValue, "URL encoding happens at the transport layer, not here")

	t.Run("missing param_name", func(t *testing.T) {
		_, err := computeAuthApplication(
			&cschema.ApiKeyPlacement{Type: cschema.ApiKeyPlacementQuery},
			database.ApiKeyCredentialPlaintext{ApiKey: "abc"},
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "param_name")
	})
}

func TestComputeAuthApplication_Basic(t *testing.T) {
	app, err := computeAuthApplication(
		&cschema.ApiKeyPlacement{
			Type:          cschema.ApiKeyPlacementBasic,
			UsernameField: "account_id",
		},
		database.ApiKeyCredentialPlaintext{ApiKey: "pass", Username: "user"},
	)
	require.NoError(t, err)
	assert.Equal(t, "Authorization", app.HeaderName)

	// Verify the encoding matches RFC 7617: base64(userid:password).
	expected := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:pass"))
	assert.Equal(t, expected, app.HeaderValue)

	t.Run("missing username", func(t *testing.T) {
		_, err := computeAuthApplication(
			&cschema.ApiKeyPlacement{
				Type:          cschema.ApiKeyPlacementBasic,
				UsernameField: "account_id",
			},
			database.ApiKeyCredentialPlaintext{ApiKey: "pass"},
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "username")
	})

	t.Run("colon in username does not corrupt encoding", func(t *testing.T) {
		// Username with a colon would be ambiguous on decode, but base64 encodes
		// the whole concatenated string verbatim — the server-side decoder gets
		// the raw bytes back, and parsing convention says everything before the
		// FIRST colon is the user. Asserting that we don't pre-split or escape.
		app, err := computeAuthApplication(
			&cschema.ApiKeyPlacement{
				Type:          cschema.ApiKeyPlacementBasic,
				UsernameField: "u",
			},
			database.ApiKeyCredentialPlaintext{ApiKey: "pw", Username: "user:with:colons"},
		)
		require.NoError(t, err)
		decoded, derr := base64.StdEncoding.DecodeString(app.HeaderValue[len("Basic "):])
		require.NoError(t, derr)
		assert.Equal(t, "user:with:colons:pw", string(decoded))
	})
}

func TestComputeAuthApplication_Errors(t *testing.T) {
	t.Run("nil placement", func(t *testing.T) {
		_, err := computeAuthApplication(nil, database.ApiKeyCredentialPlaintext{ApiKey: "abc"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "placement")
	})

	t.Run("empty api_key", func(t *testing.T) {
		_, err := computeAuthApplication(
			&cschema.ApiKeyPlacement{Type: cschema.ApiKeyPlacementBearer},
			database.ApiKeyCredentialPlaintext{},
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty")
	})

	t.Run("unknown placement type", func(t *testing.T) {
		_, err := computeAuthApplication(
			&cschema.ApiKeyPlacement{Type: cschema.ApiKeyPlacementType("aws-sigv4")},
			database.ApiKeyCredentialPlaintext{ApiKey: "abc"},
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported")
	})
}
