package jwt

import (
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func pathToTestData(path string) string {
	return "../../../test_data/" + path
}

func TestJwtTokenBuilder(t *testing.T) {
	t.Parallel()
	t.Run("getSigningKeyDataAndMethod", func(t *testing.T) {
		t.Run("RSA SSH", func(t *testing.T) {
			tb := NewJwtTokenBuilder().
				WithPrivateKeyPath(pathToTestData("admin_user_keys/ronaldreagan-ssh-rsa"))
			x := tb.(*tokenBuilder)
			_, signingMethod, err := x.getSigningKeyDataAndMethod()
			require.NoError(t, err)
			require.IsType(t, jwt.SigningMethodRS256, signingMethod)
		})
		t.Run("RSA PEM", func(t *testing.T) {
			tb := NewJwtTokenBuilder().
				WithPrivateKeyPath(pathToTestData("admin_user_keys/ronaldreagan-pem-rsa.pem"))
			x := tb.(*tokenBuilder)
			_, signingMethod, err := x.getSigningKeyDataAndMethod()
			require.NoError(t, err)
			require.IsType(t, jwt.SigningMethodRS256, signingMethod)
		})
		t.Run("ed SSH", func(t *testing.T) {
			tb := NewJwtTokenBuilder().
				WithPrivateKeyPath(pathToTestData("admin_user_keys/georgebush-ssh-ed"))
			x := tb.(*tokenBuilder)
			_, signingMethod, err := x.getSigningKeyDataAndMethod()
			require.NoError(t, err)
			require.IsType(t, jwt.SigningMethodEdDSA, signingMethod)
		})
		t.Run("ed PEM", func(t *testing.T) {
			tb := NewJwtTokenBuilder().
				WithPrivateKeyPath(pathToTestData("admin_user_keys/georgebush-pem-ed.pem"))
			x := tb.(*tokenBuilder)
			_, signingMethod, err := x.getSigningKeyDataAndMethod()
			require.NoError(t, err)
			require.IsType(t, jwt.SigningMethodEdDSA, signingMethod)
		})
		t.Run("ec SSH", func(t *testing.T) {
			tb := NewJwtTokenBuilder().
				WithPrivateKeyPath(pathToTestData("admin_user_keys/jimmycarter-ssh-ec"))
			x := tb.(*tokenBuilder)
			_, signingMethod, err := x.getSigningKeyDataAndMethod()
			require.NoError(t, err)
			_, ok := signingMethod.(*jwt.SigningMethodECDSA)
			require.True(t, ok)
		})
		t.Run("ec PEM", func(t *testing.T) {
			tb := NewJwtTokenBuilder().
				WithPrivateKeyPath(pathToTestData("admin_user_keys/jimmycarter-pem-ec.pem"))
			x := tb.(*tokenBuilder)
			_, signingMethod, err := x.getSigningKeyDataAndMethod()
			require.NoError(t, err)
			_, ok := signingMethod.(*jwt.SigningMethodECDSA)
			require.True(t, ok)

			tb = NewJwtTokenBuilder().
				WithPrivateKeyPath(pathToTestData("admin_user_keys/jimmycarter-pem-ec-old.pem"))
			x = tb.(*tokenBuilder)
			_, signingMethod, err = x.getSigningKeyDataAndMethod()
			require.NoError(t, err)
			_, ok = signingMethod.(*jwt.SigningMethodECDSA)
			require.True(t, ok)
		})
	})
	t.Run("labels", func(t *testing.T) {
		tb := NewJwtTokenBuilder().
			WithActorExternalId("bob-dole").
			WithNamespace("root.child").
			WithLabels(map[string]string{"foo": "bar"}).
			WithLabel("baz", "qux")

		x := tb.(*tokenBuilder)
		claims, err := x.jwtBuilder.Build()
		require.NoError(t, err)
		require.NotNil(t, claims.Actor)
		require.Equal(t, map[string]string{"foo": "bar", "baz": "qux"}, claims.Actor.Labels)
	})
}
