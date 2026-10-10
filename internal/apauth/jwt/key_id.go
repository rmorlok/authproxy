package jwt

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

// KeyIdHeader is the JOSE protected-header parameter that identifies the key used to sign a JWT.
const KeyIdHeader = "kid"

// keyIdPrefix versions the kid derivation scheme so it can change without ambiguity.
const keyIdPrefix = "ap1."

// keyIdHashBytes is the number of digest bytes retained in a kid. 128 bits is ample to avoid collisions between
// the handful of keys that are candidates for any given token.
const keyIdHashBytes = 16

var (
	// ErrUnknownKeyId indicates a token carried a kid that does not identify any trusted key version.
	ErrUnknownKeyId = errors.New("jwt kid does not match a trusted key")

	// ErrSigningMethodMismatch indicates the token's alg is not compatible with the selected verification key.
	ErrSigningMethodMismatch = errors.New("jwt signing method does not match key")
)

// KeyIdForSharedKey derives the opaque kid for an HMAC shared secret. The kid is an HMAC of a fixed label so it
// reveals nothing beyond what a token signature already does. Each version of a key has different material, and
// therefore a different kid, which lets verifiers pick the exact version during rotation.
func KeyIdForSharedKey(secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("authproxy-jwt-kid:shared"))
	return keyIdPrefix + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)[:keyIdHashBytes])
}

// KeyIdForPublicKey derives the opaque kid for an asymmetric key from its public half, so the signer (holding the
// private key) and the verifier (holding the public key) compute the same value.
func KeyIdForPublicKey(publicKey interface{}) (string, error) {
	if pk, ok := publicKey.(*ed25519.PublicKey); ok {
		publicKey = *pk
	}

	der, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return "", fmt.Errorf("failed to marshal public key for kid: %w", err)
	}

	h := sha256.New()
	h.Write([]byte("authproxy-jwt-kid:public:"))
	h.Write(der)
	return keyIdPrefix + base64.RawURLEncoding.EncodeToString(h.Sum(nil)[:keyIdHashBytes]), nil
}

// keyIdForSigningKey derives the kid for key material that has been loaded for signing.
func keyIdForSigningKey(signingKey interface{}) (string, error) {
	switch k := signingKey.(type) {
	case []byte:
		return KeyIdForSharedKey(k), nil
	case crypto.Signer:
		return KeyIdForPublicKey(k.Public())
	default:
		return "", fmt.Errorf("unsupported signing key type %T for kid", signingKey)
	}
}

// keyIdForVerifyingKey derives the kid for key material that has been loaded for verification.
func keyIdForVerifyingKey(verifyingKey interface{}) (string, error) {
	if b, ok := verifyingKey.([]byte); ok {
		return KeyIdForSharedKey(b), nil
	}

	return KeyIdForPublicKey(verifyingKey)
}

// signingMethodCompatible checks that the alg a token claims is in the family of the key selected to verify it. The
// key, never the token header, determines which algorithms are acceptable, preventing algorithm/key confusion.
func signingMethodCompatible(method jwt.SigningMethod, verifyingKey interface{}) bool {
	switch verifyingKey.(type) {
	case []byte:
		_, ok := method.(*jwt.SigningMethodHMAC)
		return ok
	case *rsa.PublicKey:
		switch method.(type) {
		case *jwt.SigningMethodRSA, *jwt.SigningMethodRSAPSS:
			return true
		}
		return false
	case *ecdsa.PublicKey:
		_, ok := method.(*jwt.SigningMethodECDSA)
		return ok
	case ed25519.PublicKey, *ed25519.PublicKey:
		_, ok := method.(*jwt.SigningMethodEd25519)
		return ok
	default:
		return false
	}
}
