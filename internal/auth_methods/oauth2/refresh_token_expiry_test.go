package oauth2

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/common"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpiresInSeconds_Unmarshal(t *testing.T) {
	tests := []struct {
		name        string
		json        string
		wantPresent bool
		wantSeconds int64
		wantErr     bool
	}{
		{name: "number", json: `15897600`, wantPresent: true, wantSeconds: 15897600},
		{name: "float", json: `3600.0`, wantPresent: true, wantSeconds: 3600},
		{name: "numeric string", json: `"8726400"`, wantPresent: true, wantSeconds: 8726400},
		{name: "null", json: `null`},
		{name: "empty string", json: `""`},
		{name: "garbage", json: `"soon"`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var v expiresInSeconds
			err := json.Unmarshal([]byte(tt.json), &v)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantPresent, v.Present)
			assert.Equal(t, tt.wantSeconds, v.Seconds)
		})
	}
}

// TestTokenResponse_RefreshTokenLifetime pins the provider field names and
// the "earliest deadline wins" rule. Sample bodies follow each provider's
// documented response shape (see refresh_token_expiry.go for sources).
func TestTokenResponse_RefreshTokenLifetime(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		want   time.Duration
		wantOK bool
	}{
		{
			name: "no refresh token lifetime (RFC 6749 only)",
			body: `{"access_token":"a","expires_in":3600,"refresh_token":"r"}`,
		},
		{
			name:   "GitHub App user token",
			body:   `{"access_token":"ghu_x","expires_in":28800,"refresh_token":"ghr_x","refresh_token_expires_in":15897600,"scope":"","token_type":"bearer"}`,
			want:   15897600 * time.Second,
			wantOK: true,
		},
		{
			// LinkedIn documents the field in seconds with a 1-year default TTL;
			// its own example values are illustrative, so a full year is used.
			name:   "LinkedIn programmatic refresh token",
			body:   `{"access_token":"AQX","expires_in":5184000,"refresh_token":"AQW","refresh_token_expires_in":31536000,"scope":"r_basicprofile"}`,
			want:   31536000 * time.Second,
			wantOK: true,
		},
		{
			name:   "Intuit QuickBooks",
			body:   `{"token_type":"bearer","expires_in":3600,"refresh_token":"AB1","x_refresh_token_expires_in":8726400,"access_token":"eyJ"}`,
			want:   8726400 * time.Second,
			wantOK: true,
		},
		{
			name:   "IETF draft idle timeout ignores authorization_expires_in",
			body:   `{"access_token":"a","refresh_token":"r","refresh_token_timeout":604800,"authorization_expires_in":2629800}`,
			want:   604800 * time.Second,
			wantOK: true,
		},
		{
			name:   "earliest of several wins",
			body:   `{"access_token":"a","refresh_token":"r","refresh_token_expires_in":31536000,"refresh_token_timeout":604800}`,
			want:   604800 * time.Second,
			wantOK: true,
		},
		{
			name: "non-positive treated as absent",
			body: `{"access_token":"a","refresh_token":"r","refresh_token_expires_in":0}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resp tokenResponse
			require.NoError(t, json.Unmarshal([]byte(tt.body), &resp))
			got, ok := resp.refreshTokenLifetime()
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRefreshTokenExpiry(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	withLifetime := func(seconds int64) *tokenResponse {
		return &tokenResponse{RefreshTokenExpiresIn: expiresInSeconds{Seconds: seconds, Present: true}}
	}
	persist := tokenPersistOptions{PersistRefreshToken: true}

	newConn := func(token cschema.AuthOauth2Token) *oAuth2Connection {
		return &oAuth2Connection{auth: &cschema.AuthOAuth2{Token: token}}
	}

	t.Run("unknown lifetime", func(t *testing.T) {
		got := newConn(cschema.AuthOauth2Token{}).refreshTokenExpiry(now, &tokenResponse{}, persist, true, nil)
		assert.Equal(t, database.OAuth2RefreshTokenExpiry{}, got)
	})

	t.Run("initial exchange schedules refresh one lead time before expiry", func(t *testing.T) {
		got := newConn(cschema.AuthOauth2Token{}).refreshTokenExpiry(now, withLifetime(180*86400), persist, true, nil)
		require.NotNil(t, got.ExpiresAt)
		require.NotNil(t, got.RefreshAt)
		assert.Equal(t, now.Add(180*day), *got.ExpiresAt)
		assert.Equal(t, now.Add(179*day), *got.RefreshAt, "default lead is 24h")
	})

	t.Run("configured lead time", func(t *testing.T) {
		got := newConn(cschema.AuthOauth2Token{
			RefreshTimeBeforeRefreshTokenExpiry: &common.HumanDuration{Duration: 7 * day},
		}).refreshTokenExpiry(now, withLifetime(30*86400), persist, true, nil)
		assert.Equal(t, now.Add(23*day), *got.RefreshAt)
	})

	t.Run("lead capped at half the remaining lifetime", func(t *testing.T) {
		got := newConn(cschema.AuthOauth2Token{}).refreshTokenExpiry(now, withLifetime(3600), persist, true, nil)
		assert.Equal(t, now.Add(30*time.Minute), *got.RefreshAt)
	})

	t.Run("background refresh disabled records expiry only", func(t *testing.T) {
		got := newConn(cschema.AuthOauth2Token{RefreshInBackground: util.ToPtr(false)}).
			refreshTokenExpiry(now, withLifetime(86400), persist, true, nil)
		assert.NotNil(t, got.ExpiresAt)
		assert.Nil(t, got.RefreshAt)
	})

	t.Run("client credentials grant ignores lifetimes", func(t *testing.T) {
		got := newConn(cschema.AuthOauth2Token{}).
			refreshTokenExpiry(now, withLifetime(86400), tokenPersistOptions{PersistRefreshToken: false}, false, nil)
		assert.Equal(t, database.OAuth2RefreshTokenExpiry{}, got)
	})

	t.Run("rotating refresh extends expiry and reschedules", func(t *testing.T) {
		// GitHub: each refresh issues a new refresh token with a fresh
		// six-month lifetime.
		prev := &database.OAuth2Token{RefreshTokenExpiresAt: util.ToPtr(now.Add(day))}
		got := newConn(cschema.AuthOauth2Token{}).refreshTokenExpiry(now, withLifetime(15897600), persist, true, prev)
		assert.Equal(t, now.Add(15897600*time.Second), *got.ExpiresAt)
		require.NotNil(t, got.RefreshAt)
	})

	t.Run("refresh that does not extend expiry stops proactive refresh", func(t *testing.T) {
		// LinkedIn: refresh_token_expires_in counts down to the original
		// one-year deadline; refreshing again would not help.
		deadline := now.Add(5 * day)
		prev := &database.OAuth2Token{RefreshTokenExpiresAt: util.ToPtr(deadline.Add(-2 * time.Second))}
		got := newConn(cschema.AuthOauth2Token{}).refreshTokenExpiry(now, withLifetime(5*86400), persist, true, prev)
		assert.Equal(t, deadline, *got.ExpiresAt)
		assert.Nil(t, got.RefreshAt)
	})

	t.Run("carried-forward refresh token keeps its known expiry", func(t *testing.T) {
		deadline := now.Add(10 * day)
		prev := &database.OAuth2Token{RefreshTokenExpiresAt: util.ToPtr(deadline)}
		got := newConn(cschema.AuthOauth2Token{}).refreshTokenExpiry(now, &tokenResponse{}, persist, false, prev)
		assert.Equal(t, deadline, *got.ExpiresAt)
		assert.Nil(t, got.RefreshAt, "a refresh that kept the same refresh token did not extend it")
	})

	t.Run("new refresh token without lifetime forgets old expiry", func(t *testing.T) {
		prev := &database.OAuth2Token{RefreshTokenExpiresAt: util.ToPtr(now.Add(day))}
		got := newConn(cschema.AuthOauth2Token{}).refreshTokenExpiry(now, &tokenResponse{}, persist, true, prev)
		assert.Equal(t, database.OAuth2RefreshTokenExpiry{}, got)
	})
}

// TestRefresh_PersistsRefreshTokenExpiry runs a real refresh against the
// rotating provider returning GitHub's refresh_token_expires_in and checks
// the persisted columns.
func TestRefresh_PersistsRefreshTokenExpiry(t *testing.T) {
	provider := &rotatingTokenProvider{extraResponse: map[string]any{"refresh_token_expires_in": 15897600}}
	hr := newRotationHarness(t, provider, time.Second)

	before := time.Now()
	require.NoError(t, hr.newConn().Refresh(context.Background()))

	token, err := hr.db.GetOAuth2Token(context.Background(), hr.connection.GetId())
	require.NoError(t, err)
	require.NotNil(t, token.RefreshTokenExpiresAt)
	require.NotNil(t, token.RefreshTokenRefreshAt)
	assert.WithinDuration(t, before.Add(15897600*time.Second), *token.RefreshTokenExpiresAt, 5*time.Second)
	assert.WithinDuration(t, token.RefreshTokenExpiresAt.Add(-cschema.DefaultRefreshTimeBeforeRefreshTokenExpiry),
		*token.RefreshTokenRefreshAt, time.Second)
}
