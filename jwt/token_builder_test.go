package jwt

import (
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestJwtTokenBuilder(t *testing.T) {
	t.Run("getSigningKeyDataAndMethod", func(t *testing.T) {
		t.Run("RSA SSH", func(t *testing.T) {
			tb := NewJwtTokenBuilder().
				WithPrivateKeyPath("../test_data/admin_user_keys/ronaldreagan-ssh-rsa")
			x := tb.(*tokenBuilder)
			_, signingMethod, err := x.getSigningKeyDataAndMethod()
			require.NoError(t, err)
			require.IsType(t, jwt.SigningMethodRS256, signingMethod)
		})
		t.Run("RSA PEM", func(t *testing.T) {
			tb := NewJwtTokenBuilder().
				WithPrivateKeyPath("../test_data/admin_user_keys/ronaldreagan-pem-rsa.pem")
			x := tb.(*tokenBuilder)
			_, signingMethod, err := x.getSigningKeyDataAndMethod()
			require.NoError(t, err)
			require.IsType(t, jwt.SigningMethodRS256, signingMethod)
		})
		t.Run("ed SSH", func(t *testing.T) {
			tb := NewJwtTokenBuilder().
				WithPrivateKeyPath("../test_data/admin_user_keys/georgebush-ssh-ed")
			x := tb.(*tokenBuilder)
			_, signingMethod, err := x.getSigningKeyDataAndMethod()
			require.NoError(t, err)
			require.IsType(t, jwt.SigningMethodEdDSA, signingMethod)
		})
		t.Run("ed PEM", func(t *testing.T) {
			tb := NewJwtTokenBuilder().
				WithPrivateKeyPath("../test_data/admin_user_keys/georgebush-pem-ed.pem")
			x := tb.(*tokenBuilder)
			_, signingMethod, err := x.getSigningKeyDataAndMethod()
			require.NoError(t, err)
			require.IsType(t, jwt.SigningMethodEdDSA, signingMethod)
		})
		t.Run("ec SSH", func(t *testing.T) {
			tb := NewJwtTokenBuilder().
				WithPrivateKeyPath("../test_data/admin_user_keys/jimmycarter-ssh-ec")
			x := tb.(*tokenBuilder)
			_, signingMethod, err := x.getSigningKeyDataAndMethod()
			require.NoError(t, err)
			_, ok := signingMethod.(*jwt.SigningMethodECDSA)
			require.True(t, ok)
		})
		t.Run("ec PEM", func(t *testing.T) {
			tb := NewJwtTokenBuilder().
				WithPrivateKeyPath("../test_data/admin_user_keys/jimmycarter-pem-ec.pem")
			x := tb.(*tokenBuilder)
			_, signingMethod, err := x.getSigningKeyDataAndMethod()
			require.NoError(t, err)
			_, ok := signingMethod.(*jwt.SigningMethodECDSA)
			require.True(t, ok)

			tb = NewJwtTokenBuilder().
				WithPrivateKeyPath("../test_data/admin_user_keys/jimmycarter-pem-ec-old.pem")
			x = tb.(*tokenBuilder)
			_, signingMethod, err = x.getSigningKeyDataAndMethod()
			require.NoError(t, err)
			_, ok = signingMethod.(*jwt.SigningMethodECDSA)
			require.True(t, ok)
		})
	})
}
