package jwt

import (
	"context"
	"os"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/resources/key"
	"github.com/stretchr/testify/require"
)

func unverifiedKeyId(t *testing.T, token string) (string, bool) {
	t.Helper()
	parsed, _, err := jwt.NewParser().ParseUnverified(token, &AuthProxyClaims{})
	require.NoError(t, err)
	kid, ok := parsed.Header[KeyIdHeader].(string)
	return kid, ok
}

// signWithHeader signs claims directly, bypassing the token builder, so tests can control the kid header.
func signWithHeader(t *testing.T, method jwt.SigningMethod, signingKey interface{}, kid interface{}) string {
	t.Helper()
	token := jwt.NewWithClaims(method, &AuthProxyClaims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: "bobdole"},
	})
	if kid != nil {
		token.Header[KeyIdHeader] = kid
	}
	s, err := token.SignedString(signingKey)
	require.NoError(t, err)
	return s
}

func TestKeyId(t *testing.T) {
	t.Parallel()

	t.Run("asymmetric key pairs produce matching kids", func(t *testing.T) {
		for _, tt := range []struct {
			name    string
			private string
			public  string
		}{
			{"RSA SSH", "ronaldreagan-ssh-rsa", "ronaldreagan-ssh-rsa.pub"},
			{"RSA PEM", "ronaldreagan-pem-rsa.pem", "ronaldreagan-pem-rsa-pub.pem"},
			{"ed SSH", "georgebush-ssh-ed", "georgebush-ssh-ed.pub"},
			{"ed PEM", "georgebush-pem-ed.pem", "georgebush-pem-ed-pub.pem"},
			{"ec SSH", "jimmycarter-ssh-ec", "jimmycarter-ssh-ec.pub"},
			{"ec PEM", "jimmycarter-pem-ec.pem", "jimmycarter-pem-ec-pub.pem"},
		} {
			t.Run(tt.name, func(t *testing.T) {
				token, err := NewJwtTokenBuilder().
					WithActorExternalId("bobdole").
					WithPrivateKeyPath(pathToTestData("admin_user_keys/" + tt.private)).
					Token()
				require.NoError(t, err)

				kid, ok := unverifiedKeyId(t, token)
				require.True(t, ok)
				require.Regexp(t, `^ap1\.[A-Za-z0-9_-]{22}$`, kid)

				pubData, err := os.ReadFile(pathToTestData("admin_user_keys/" + tt.public))
				require.NoError(t, err)
				pub, _, err := loadPublicKeyFromPEMOrOpenSSH(pubData)
				require.NoError(t, err)
				pubKid, err := keyIdForVerifyingKey(pub)
				require.NoError(t, err)
				require.Equal(t, kid, pubKid)

				claims, err := NewJwtTokenParserBuilder().
					WithPublicKeyPath(pathToTestData("admin_user_keys/" + tt.public)).
					Parse(token)
				require.NoError(t, err)
				require.Equal(t, "bobdole", claims.Subject)
			})
		}
	})

	t.Run("shared keys", func(t *testing.T) {
		token := NewJwtTokenBuilder().
			WithActorExternalId("bobdole").
			WithSecretKey([]byte("secret-one")).
			MustToken()

		kid, ok := unverifiedKeyId(t, token)
		require.True(t, ok)
		require.Equal(t, KeyIdForSharedKey([]byte("secret-one")), kid)
		require.NotEqual(t, KeyIdForSharedKey([]byte("secret-two")), kid)

		claims, err := NewJwtTokenParserBuilder().WithSharedKeyString("secret-one").Parse(token)
		require.NoError(t, err)
		require.Equal(t, "bobdole", claims.Subject)
	})

	t.Run("different keys produce different kids", func(t *testing.T) {
		kid1 := unverifiedKeyIdForPrivateKey(t, "admin_user_keys/bobdole")
		kid2 := unverifiedKeyIdForPrivateKey(t, "admin_user_keys/billclinton")
		require.NotEqual(t, kid1, kid2)
	})
}

func unverifiedKeyIdForPrivateKey(t *testing.T, path string) string {
	t.Helper()
	token, err := NewJwtTokenBuilder().
		WithActorExternalId("bobdole").
		WithPrivateKeyPath(pathToTestData(path)).
		Token()
	require.NoError(t, err)
	kid, ok := unverifiedKeyId(t, token)
	require.True(t, ok)
	return kid
}

func TestParseKeyId(t *testing.T) {
	t.Parallel()
	secret := []byte("a-long-enough-test-secret")
	parser := func() ParserBuilder { return NewJwtTokenParserBuilder().WithSharedKey(secret) }

	t.Run("legacy token without kid", func(t *testing.T) {
		token := signWithHeader(t, jwt.SigningMethodHS256, secret, nil)
		claims, err := parser().Parse(token)
		require.NoError(t, err)
		require.Equal(t, "bobdole", claims.Subject)
	})

	t.Run("legacy asymmetric token without kid", func(t *testing.T) {
		keyData, err := os.ReadFile(pathToTestData("admin_user_keys/bobdole"))
		require.NoError(t, err)
		signingKey, method, err := loadPrivateKeyFromPEMOrOpenSSH(keyData)
		require.NoError(t, err)

		token := signWithHeader(t, method, signingKey, nil)
		claims, err := NewJwtTokenParserBuilder().
			WithPublicKeyPath(pathToTestData("admin_user_keys/bobdole.pub")).
			Parse(token)
		require.NoError(t, err)
		require.Equal(t, "bobdole", claims.Subject)
	})

	t.Run("matching kid", func(t *testing.T) {
		token := signWithHeader(t, jwt.SigningMethodHS256, secret, KeyIdForSharedKey(secret))
		_, err := parser().Parse(token)
		require.NoError(t, err)
	})

	t.Run("unknown kid", func(t *testing.T) {
		token := signWithHeader(t, jwt.SigningMethodHS256, secret, "ap1.unknown")
		_, err := parser().Parse(token)
		require.ErrorIs(t, err, ErrUnknownKeyId)
	})

	t.Run("empty kid", func(t *testing.T) {
		token := signWithHeader(t, jwt.SigningMethodHS256, secret, "")
		_, err := parser().Parse(token)
		require.ErrorIs(t, err, ErrUnknownKeyId)
	})

	t.Run("non-string kid", func(t *testing.T) {
		token := signWithHeader(t, jwt.SigningMethodHS256, secret, 12345)
		_, err := parser().Parse(token)
		require.ErrorIs(t, err, ErrUnknownKeyId)
	})

	t.Run("kid of a different key", func(t *testing.T) {
		token := signWithHeader(t, jwt.SigningMethodHS256, secret, KeyIdForSharedKey([]byte("other")))
		_, err := parser().Parse(token)
		require.ErrorIs(t, err, ErrUnknownKeyId)
	})

	t.Run("public key used as HMAC secret is rejected", func(t *testing.T) {
		pubData, err := os.ReadFile(pathToTestData("admin_user_keys/bobdole.pub"))
		require.NoError(t, err)
		pub, _, err := loadPublicKeyFromPEMOrOpenSSH(pubData)
		require.NoError(t, err)
		pubKid, err := keyIdForVerifyingKey(pub)
		require.NoError(t, err)

		// Classic algorithm confusion: an attacker signs with HS256 using the public key bytes as the secret and
		// claims the kid of the public key.
		for _, kid := range []interface{}{nil, pubKid} {
			token := signWithHeader(t, jwt.SigningMethodHS256, pubData, kid)
			_, err = NewJwtTokenParserBuilder().
				WithPublicKeyPath(pathToTestData("admin_user_keys/bobdole.pub")).
				Parse(token)
			require.ErrorIs(t, err, ErrSigningMethodMismatch)
		}
	})
}

func TestParseKeyIdRotation(t *testing.T) {
	ctx := context.Background()
	t.Cleanup(key.ResetKeyDataMockRegistry)

	const mockId = "jwt-kid-rotation"
	oldSecret := []byte("old-secret-old-secret-old-secret")
	newSecret := []byte("new-secret-new-secret-new-secret")
	key.NewKeyDataMock(mockId)
	key.KeyDataMockAddVersion(mockId, "jwt", "v1", oldSecret)
	key.KeyDataMockAddVersion(mockId, "jwt", "v2", newSecret)

	cfgKey := &config.Key{InnerVal: &config.KeyShared{SharedKey: &config.KeyData{InnerVal: &key.KeyDataMock{MockID: mockId}}}}
	parser := func() ParserBuilder { return NewJwtTokenParserBuilder().WithConfigKey(ctx, cfgKey) }

	t.Run("new tokens are signed with the current version", func(t *testing.T) {
		tb, err := NewJwtTokenBuilder().WithActorExternalId("bobdole").WithConfigKey(ctx, cfgKey)
		require.NoError(t, err)
		token := tb.MustToken()

		kid, ok := unverifiedKeyId(t, token)
		require.True(t, ok)
		require.Equal(t, KeyIdForSharedKey(newSecret), kid)

		_, err = parser().Parse(token)
		require.NoError(t, err)
	})

	t.Run("previous version is selected by kid", func(t *testing.T) {
		token := signWithHeader(t, jwt.SigningMethodHS256, oldSecret, KeyIdForSharedKey(oldSecret))
		claims, err := parser().Parse(token)
		require.NoError(t, err)
		require.Equal(t, "bobdole", claims.Subject)
	})

	t.Run("legacy token from previous version is not accepted", func(t *testing.T) {
		token := signWithHeader(t, jwt.SigningMethodHS256, oldSecret, nil)
		_, err := parser().Parse(token)
		require.Error(t, err)
	})

	t.Run("kid of one version with signature from another", func(t *testing.T) {
		token := signWithHeader(t, jwt.SigningMethodHS256, oldSecret, KeyIdForSharedKey(newSecret))
		_, err := parser().Parse(token)
		require.ErrorIs(t, err, jwt.ErrTokenSignatureInvalid)
	})

	t.Run("removed version is unknown", func(t *testing.T) {
		key.KeyDataMockRemoveVersion(mockId, "v1")
		token := signWithHeader(t, jwt.SigningMethodHS256, oldSecret, KeyIdForSharedKey(oldSecret))
		_, err := parser().Parse(token)
		require.ErrorIs(t, err, ErrUnknownKeyId)
	})
}
