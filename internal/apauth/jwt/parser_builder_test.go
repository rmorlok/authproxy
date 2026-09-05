package jwt

import (
	"context"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rmorlok/authproxy/internal/apauth/core"
	authschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/stretchr/testify/require"
)

func TestJwtTokenParserBuilder(t *testing.T) {
	t.Parallel()
	t.Run("getSigningKeyDataAndMethod", func(t *testing.T) {
		t.Run("RSA SSH", func(t *testing.T) {
			tb := NewJwtTokenParserBuilder().
				WithPublicKeyPath(pathToTestData("admin_user_keys/ronaldreagan-ssh-rsa.pub"))
			x := tb.(*parserBuilder)
			_, signingMethod, err := x.getVerifyingKeyData(context.Background(), nil)
			require.NoError(t, err)
			require.IsType(t, jwt.SigningMethodRS256, signingMethod)
		})
		t.Run("RSA PEM", func(t *testing.T) {
			tb := NewJwtTokenParserBuilder().
				WithPublicKeyPath(pathToTestData("admin_user_keys/ronaldreagan-pem-rsa-pub.pem"))
			x := tb.(*parserBuilder)
			_, signingMethod, err := x.getVerifyingKeyData(context.Background(), nil)
			require.NoError(t, err)
			require.IsType(t, jwt.SigningMethodRS256, signingMethod)
		})
		t.Run("ed SSH", func(t *testing.T) {
			tb := NewJwtTokenParserBuilder().
				WithPublicKeyPath(pathToTestData("admin_user_keys/georgebush-ssh-ed.pub"))
			x := tb.(*parserBuilder)
			_, signingMethod, err := x.getVerifyingKeyData(context.Background(), nil)
			require.NoError(t, err)
			require.IsType(t, jwt.SigningMethodEdDSA, signingMethod)
		})
		t.Run("ed PEM", func(t *testing.T) {
			tb := NewJwtTokenParserBuilder().
				WithPublicKeyPath(pathToTestData("admin_user_keys/georgebush-pem-ed-pub.pem"))
			x := tb.(*parserBuilder)
			_, signingMethod, err := x.getVerifyingKeyData(context.Background(), nil)
			require.NoError(t, err)
			require.IsType(t, jwt.SigningMethodEdDSA, signingMethod)
		})
		t.Run("ec SSH", func(t *testing.T) {
			tb := NewJwtTokenParserBuilder().
				WithPublicKeyPath(pathToTestData("admin_user_keys/jimmycarter-ssh-ec.pub"))
			x := tb.(*parserBuilder)
			_, signingMethod, err := x.getVerifyingKeyData(context.Background(), nil)
			require.NoError(t, err)
			_, ok := signingMethod.(*jwt.SigningMethodECDSA)
			require.True(t, ok)
		})
		t.Run("ec PEM", func(t *testing.T) {
			tb := NewJwtTokenParserBuilder().
				WithPublicKeyPath(pathToTestData("admin_user_keys/jimmycarter-pem-ec-pub.pem"))
			x := tb.(*parserBuilder)
			_, signingMethod, err := x.getVerifyingKeyData(context.Background(), nil)
			require.NoError(t, err)
			_, ok := signingMethod.(*jwt.SigningMethodECDSA)
			require.True(t, ok)
		})
	})
}

func TestJwtTokenParserBuilderActorResource(t *testing.T) {
	secret := []byte("a-long-enough-test-secret")
	basePermissions := []authschema.Permission{{
		Namespace: "root.platform.**",
		Resources: []string{"connections"},
		Verbs:     []string{"get", "list"},
	}}
	token, err := NewJwtTokenBuilder().
		WithActor(&core.Actor{
			Name:        "deploy-bot",
			ExternalId:  "deploy-bot",
			Namespace:   "root.platform",
			Permissions: basePermissions,
		}).
		WithPermissions([]authschema.Permission{{
			Namespace: "root.platform.production",
			Resources: []string{"connections"},
			Verbs:     []string{"get"},
		}}).
		WithSecretKey(secret).
		Token()
	require.NoError(t, err)

	claims, err := NewJwtTokenParserBuilder().WithSharedKey(secret).Parse(token)
	require.NoError(t, err)
	require.NoError(t, claims.Validate(jwt.NewValidator()))
	require.Equal(t, "authproxy.net/v1alpha1", string(claims.Actor.APIVersion))
	require.Equal(t, "Actor", string(claims.Actor.Kind))
	require.Equal(t, "deploy-bot", claims.Actor.Spec.ExternalId)
	require.Equal(t, "root.platform", claims.Actor.Metadata.Namespace)
	require.Empty(t, claims.Actor.Metadata.ID)
	require.Nil(t, claims.Actor.Spec.SigningKey)
	require.Nil(t, claims.Actor.Status)
}

func TestJwtTokenParserBuilderRejectsLegacyActor(t *testing.T) {
	secret := []byte("a-long-enough-test-secret")
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":       "legacy",
		"namespace": "root",
		"actor": map[string]any{
			"externalId":  "legacy",
			"namespace":   "root",
			"permissions": []any{},
		},
	})
	signed, err := token.SignedString(secret)
	require.NoError(t, err)

	_, err = NewJwtTokenParserBuilder().WithSharedKey(secret).Parse(signed)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown field")
}

func TestJwtTokenParserBuilderRejectsActorClaimMismatches(t *testing.T) {
	secret := []byte("a-long-enough-test-secret")
	basePermissions := []authschema.Permission{{
		Namespace: "root.platform.**",
		Resources: []string{"connections"},
		Verbs:     []string{"get"},
	}}
	newClaims := func() *AuthProxyClaims {
		return &AuthProxyClaims{
			RegisteredClaims: jwt.RegisteredClaims{Subject: "deploy-bot"},
			Namespace:        "root.platform",
			Actor: NewActorClaim(&core.Actor{
				ExternalId:  "deploy-bot",
				Namespace:   "root.platform",
				Permissions: basePermissions,
			}),
		}
	}

	tests := map[string]struct {
		mutate    func(*AuthProxyClaims)
		errorText string
	}{
		"subject": {
			mutate:    func(claims *AuthProxyClaims) { claims.Subject = "other" },
			errorText: "subject and actor spec.externalId",
		},
		"namespace": {
			mutate:    func(claims *AuthProxyClaims) { claims.Namespace = "root.other" },
			errorText: "namespace and actor metadata.namespace",
		},
		"permissions": {
			mutate: func(claims *AuthProxyClaims) {
				claims.Permissions = []authschema.Permission{{
					Namespace: "root.**",
					Resources: []string{"actors"},
					Verbs:     []string{"delete"},
				}}
			},
			errorText: "beyond the actor's permissions",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			claims := newClaims()
			tt.mutate(claims)
			signed, err := NewJwtTokenBuilder().
				WithClaims(claims).
				WithSecretKey(secret).
				Token()
			require.NoError(t, err)

			_, err = NewJwtTokenParserBuilder().WithSharedKey(secret).Parse(signed)
			require.ErrorIs(t, err, ErrInvalidClaims)
			require.Contains(t, err.Error(), tt.errorText)
		})
	}
}
