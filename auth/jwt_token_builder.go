package auth

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/context"
	"golang.org/x/crypto/ssh"
	"os"
	"time"
)

// JwtTokenBuilder extends from JwtBuilder to provide options to sign tokens
type JwtTokenBuilder interface {
	WithIssuer(issuer string) JwtTokenBuilder
	WithAudience(audience string) JwtTokenBuilder
	WithServiceId(serviceId config.ServiceId) JwtTokenBuilder
	WithServiceIds(serviceId []config.ServiceId) JwtTokenBuilder
	WithExpiration(expiration time.Time) JwtTokenBuilder
	WithExpiresIn(expiresIn time.Duration) JwtTokenBuilder
	WithExpiresInCtx(ctx context.Context, expiresIn time.Duration) JwtTokenBuilder
	WithSuperAdmin() JwtTokenBuilder
	WithAdmin() JwtTokenBuilder
	WithActorEmail(email string) JwtTokenBuilder
	WithActorId(id string) JwtTokenBuilder
	WithSessionOnly() JwtTokenBuilder

	WithPrivateKeyPath(string) JwtTokenBuilder
	WithPrivateKeyString(string) JwtTokenBuilder
	WithPrivateKey([]byte) JwtTokenBuilder
	WithSecretKeyPath(string) JwtTokenBuilder
	WithSecretKeyString(string) JwtTokenBuilder
	WithSecretKey([]byte) JwtTokenBuilder

	TokenCtx(context.Context) (string, error)
	Token() (string, error)
	MustTokenCtx(context.Context) string
	MustToken() string
}

type jwtTokenBuilder struct {
	jwtBuilder     jwtBuilder
	privateKeyPath *string
	privateKeyData []byte
	secretKeyPath  *string
	secretKeyData  []byte
}

func (tb *jwtTokenBuilder) WithIssuer(issuer string) JwtTokenBuilder {
	tb.jwtBuilder.WithIssuer(issuer)
	return tb
}

func (tb *jwtTokenBuilder) WithAudience(audience string) JwtTokenBuilder {
	tb.jwtBuilder.WithAudience(audience)
	return tb
}

func (tb *jwtTokenBuilder) WithServiceId(serviceId config.ServiceId) JwtTokenBuilder {
	tb.jwtBuilder.WithServiceId(serviceId)
	return tb
}

func (tb *jwtTokenBuilder) WithServiceIds(serviceIds []config.ServiceId) JwtTokenBuilder {
	tb.jwtBuilder.WithServiceIds(serviceIds)
	return tb
}

func (tb *jwtTokenBuilder) WithExpiration(expiration time.Time) JwtTokenBuilder {
	tb.jwtBuilder.WithExpiration(expiration)
	return tb
}

func (tb *jwtTokenBuilder) WithExpiresIn(expiresIn time.Duration) JwtTokenBuilder {
	tb.jwtBuilder.WithExpiresIn(expiresIn)
	return tb
}

func (tb *jwtTokenBuilder) WithExpiresInCtx(ctx context.Context, expiresIn time.Duration) JwtTokenBuilder {
	tb.jwtBuilder.WithExpiresInCtx(ctx, expiresIn)
	return tb
}

func (tb *jwtTokenBuilder) WithSuperAdmin() JwtTokenBuilder {
	tb.jwtBuilder.WithSuperAdmin()
	return tb
}

func (tb *jwtTokenBuilder) WithAdmin() JwtTokenBuilder {
	tb.jwtBuilder.WithAdmin()
	return tb
}

func (tb *jwtTokenBuilder) WithActorEmail(email string) JwtTokenBuilder {
	tb.jwtBuilder.WithActorEmail(email)
	return tb
}

func (tb *jwtTokenBuilder) WithActorId(id string) JwtTokenBuilder {
	tb.jwtBuilder.WithActorId(id)
	return tb
}

func (tb *jwtTokenBuilder) WithSessionOnly() JwtTokenBuilder {
	tb.jwtBuilder.WithSessionOnly()
	return tb
}

func (tb *jwtTokenBuilder) WithPrivateKeyPath(privateKeyPath string) JwtTokenBuilder {
	tb.privateKeyPath = &privateKeyPath
	return tb
}

func (tb *jwtTokenBuilder) WithPrivateKeyString(privateKey string) JwtTokenBuilder {
	return tb.WithPrivateKey([]byte(privateKey))
}

func (tb *jwtTokenBuilder) WithPrivateKey(privateKey []byte) JwtTokenBuilder {
	tb.privateKeyData = privateKey
	return tb
}

func (tb *jwtTokenBuilder) WithSecretKeyPath(secretKeyPath string) JwtTokenBuilder {
	tb.secretKeyPath = &secretKeyPath
	return tb
}

func (tb *jwtTokenBuilder) WithSecretKeyString(secretKey string) JwtTokenBuilder {
	return tb.WithSecretKey([]byte(secretKey))
}

func (tb *jwtTokenBuilder) WithSecretKey(secretKey []byte) JwtTokenBuilder {
	tb.secretKeyData = secretKey
	return tb
}

func (tb *jwtTokenBuilder) getSigningMethod() jwt.SigningMethod {
	if tb.privateKeyData != nil || tb.privateKeyPath != nil {
		return jwt.SigningMethodRS256
	}

	return jwt.SigningMethodHS256
}

// loadRSAPrivateKeyFromPEM loads an RSA private key from a PEM file data
func loadRSAPrivateKeyFromPEMOrOpenSSH(keyData []byte) (*rsa.PrivateKey, error) {
	parsedKey, err := ssh.ParseRawPrivateKey(keyData)
	if err == nil {
		rsaKey, ok := parsedKey.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("not an RSA private key")
		}

		return rsaKey, nil
	}

	// Decode the PEM block
	block, _ := pem.Decode(keyData)
	if block == nil || block.Type != "RSA PRIVATE KEY" {
		return nil, fmt.Errorf("failed to decode key as OpenSSH RSA and failed to decode PEM block containing private key")
	}

	// Parse the DER-encoded RSA private key
	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse RSA private key: %w", err)
	}

	return privateKey, nil
}

func (tb *jwtTokenBuilder) getSigningKeyData() (interface{}, error) {
	if tb.privateKeyData != nil && tb.privateKeyPath != nil {
		return nil, errors.New("cannot specify secret key data and path")
	}

	if tb.secretKeyPath != nil && tb.secretKeyData != nil {
		return nil, errors.New("cannot specify secret key data and path")
	}

	if tb.privateKeyData == nil && tb.privateKeyPath == nil &&
		tb.secretKeyPath == nil && tb.secretKeyData == nil {
		return nil, errors.New("key material must be specified in some form")
	}

	if tb.privateKeyData != nil {
		return loadRSAPrivateKeyFromPEMOrOpenSSH(tb.privateKeyData)
	}

	if tb.secretKeyData != nil {
		return tb.secretKeyData, nil
	}

	pathType := "private"
	isPrivate := true
	pathPtr := tb.privateKeyPath

	if tb.secretKeyPath != nil {
		pathType = "secret"
		isPrivate = false
		pathPtr = tb.secretKeyPath
	}

	path := *pathPtr
	_, err := os.Stat(path)
	if err != nil {
		// attempt home path expansion
		path, err = homedir.Expand(path)
		if err != nil {
			return nil, err
		}
	}

	_, err = os.Stat(path)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid %s key path", pathType)
	}

	keyData, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading %s key", pathType)
	}

	if isPrivate {
		return loadRSAPrivateKeyFromPEMOrOpenSSH(keyData)
	}

	return keyData, nil
}

func (tb *jwtTokenBuilder) TokenCtx(ctx context.Context) (string, error) {
	claims, err := tb.jwtBuilder.BuildCtx(ctx)
	if err != nil {
		return "", err
	}

	keyData, err := tb.getSigningKeyData()
	if err != nil {
		return "", err
	}

	token := jwt.NewWithClaims(tb.getSigningMethod(), claims)
	tokenString, err := token.SignedString(keyData)
	if err != nil {
		return "", errors.Wrap(err, "error signing jwt")
	}

	return tokenString, nil
}

func (tb *jwtTokenBuilder) Token() (string, error) {
	return tb.TokenCtx(context.Background())
}

func (tb *jwtTokenBuilder) MustTokenCtx(ctx context.Context) string {
	token, err := tb.TokenCtx(ctx)
	if err != nil {
		panic(err)
	}

	return token
}

func (tb *jwtTokenBuilder) MustToken() string {
	return tb.MustTokenCtx(context.Background())
}

func NewJwtTokenBuilder() JwtTokenBuilder {
	return &jwtTokenBuilder{}
}
