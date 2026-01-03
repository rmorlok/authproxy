package jwt

import (
	"context"
	"testing"

	"github.com/golang-jwt/jwt/v5"
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
