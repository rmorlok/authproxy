package auth

import (
	"crypto/ecdsa"
	"crypto/ed25519"
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
	// WithClaims allows the claims to be specified explicitly instead of built progressively
	WithClaims(c *JwtTokenClaims) JwtTokenBuilder

	/*
	 * Create claims dynamically as part of the builder
	 */

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

	WithConfigKey(ctx context.Context, cfgKey config.Key) (JwtTokenBuilder, error)
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
	claims         *JwtTokenClaims
	privateKeyPath *string
	privateKeyData []byte
	secretKeyPath  *string
	secretKeyData  []byte
}

func (tb *jwtTokenBuilder) WithClaims(c *JwtTokenClaims) JwtTokenBuilder {
	tb.claims = c
	return tb
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

func (tb *jwtTokenBuilder) WithConfigKey(ctx context.Context, cfgKey config.Key) (JwtTokenBuilder, error) {
	var us JwtTokenBuilder = tb

	if pp, ok := cfgKey.(*config.KeyPublicPrivate); ok {
		if pp.PrivateKey != nil {
			if pp.PrivateKey.HasData(ctx) {
				data, err := pp.PrivateKey.GetData(ctx)
				if err != nil {
					return us, err
				}
				us = us.WithPrivateKey(data)
			}
		}
	}

	if ks, ok := cfgKey.(*config.KeyShared); ok {
		if ks.SharedKey != nil {
			if ks.SharedKey.HasData(ctx) {
				data, err := ks.SharedKey.GetData(ctx)
				if err != nil {
					return us, err
				}
				us = us.WithSecretKey(data)
			}
		}
	}

	return us, nil
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

// loadRSAPrivateKeyFromPEM loads an RSA private key from a PEM file data
func loadPrivateKeyFromPEMOrOpenSSH(keyData []byte) (interface{}, jwt.SigningMethod, error) {
	parsedKey, err := ssh.ParseRawPrivateKey(keyData)
	if err == nil {
		return signingKeyMethodFromParsedPrivateKey(parsedKey)
	}

	// Decode the PEM block
	block, rest := pem.Decode(keyData)
	if block == nil {
		return nil, nil, fmt.Errorf("failed to decode key as OpenSSH and failed to decode PEM block containing private key")
	}

	if block.Type == "EC PARAMETERS" {
		block, _ = pem.Decode(rest)
		if block == nil {
			return nil, nil, fmt.Errorf("EC PEM file contained EC PARMETERS but not EC PRIVATE KEY")
		}
	}

	switch block.Type {
	case "RSA PRIVATE KEY":
		// Parse the DER-encoded RSA private key
		privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "failed to parse RSA private key")
		}

		return privateKey, jwt.SigningMethodRS256, nil
	case "EC PRIVATE KEY":
		// Parse the EC private key
		privateKey, err := x509.ParseECPrivateKey(block.Bytes)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "failed to parse EC private key")
		}

		return signingKeyMethodFromParsedPrivateKey(privateKey)
	case "PRIVATE KEY":
		// Parse an unencrypted private key (PKCS#8 encoded)
		privateKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "failed to parse private key")
		}

		return signingKeyMethodFromParsedPrivateKey(privateKey)
	default:
		return nil, nil, fmt.Errorf("unsupported private key type: %s", block.Type)
	}
}

func signingKeyMethodFromParsedPrivateKey(parsedKey interface{}) (interface{}, jwt.SigningMethod, error) {
	switch k := parsedKey.(type) {
	case *rsa.PrivateKey:
		return parsedKey, jwt.SigningMethodRS256, nil
	case *ecdsa.PrivateKey:
		switch k.Curve.Params().Name {
		case "P-256":
			return parsedKey, jwt.SigningMethodES256, nil
		case "P-384":
			return parsedKey, jwt.SigningMethodES384, nil
		case "P-521":
			return parsedKey, jwt.SigningMethodES512, nil
		default:
			return nil, nil, errors.New("unsupported elliptic curve for ECDSA")
		}
	case *ed25519.PrivateKey:
		return parsedKey, jwt.SigningMethodEdDSA, nil
	case ed25519.PrivateKey:
		return parsedKey, jwt.SigningMethodEdDSA, nil
	default:
		return nil, nil, errors.New("unsupported private key type")
	}
}

func (tb *jwtTokenBuilder) getSigningKeyDataAndMethod() (interface{}, jwt.SigningMethod, error) {
	if tb.privateKeyData != nil && tb.privateKeyPath != nil {
		return nil, nil, errors.New("cannot specify secret key data and path")
	}

	if tb.secretKeyPath != nil && tb.secretKeyData != nil {
		return nil, nil, errors.New("cannot specify secret key data and path")
	}

	if tb.privateKeyData == nil && tb.privateKeyPath == nil &&
		tb.secretKeyPath == nil && tb.secretKeyData == nil {
		return nil, nil, errors.New("key material must be specified in some form")
	}

	if tb.privateKeyData != nil {
		return loadPrivateKeyFromPEMOrOpenSSH(tb.privateKeyData)
	}

	if tb.secretKeyData != nil {
		return tb.secretKeyData, jwt.SigningMethodHS256, nil
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
			return nil, nil, err
		}
	}

	_, err = os.Stat(path)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "invalid %s key path", pathType)
	}

	keyData, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "error reading %s key", pathType)
	}

	if isPrivate {
		return loadPrivateKeyFromPEMOrOpenSSH(keyData)
	}

	return keyData, jwt.SigningMethodHS256, nil
}

func (tb *jwtTokenBuilder) TokenCtx(ctx context.Context) (string, error) {
	var claims *JwtTokenClaims
	var err error

	if tb.claims != nil {
		claims = tb.claims
	} else {
		claims, err = tb.jwtBuilder.BuildCtx(ctx)
		if err != nil {
			return "", err
		}
	}

	keyData, signingMethdod, err := tb.getSigningKeyDataAndMethod()
	if err != nil {
		return "", err
	}

	token := jwt.NewWithClaims(signingMethdod, claims)
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
