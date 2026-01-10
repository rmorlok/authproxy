package jwt

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util"
	"golang.org/x/crypto/ssh"
)

// TokenBuilder extends from ClaimsBuilder to provide options to sign tokens
type TokenBuilder interface {
	// WithClaims allows the claims to be specified explicitly instead of built progressively
	WithClaims(c *AuthProxyClaims) TokenBuilder

	/*
	 * Create claims dynamically as part of the builder
	 */

	WithIssuer(issuer string) TokenBuilder
	WithAudience(audience string) TokenBuilder             // Specifies the audience of the claims; normally a service id
	WithServiceId(serviceId config.ServiceId) TokenBuilder // Specifies the service that is intended to consume the claims. Communicated as aud.
	WithServiceIds(serviceId []config.ServiceId) TokenBuilder
	WithExpiration(expiration time.Time) TokenBuilder
	WithExpiresIn(expiresIn time.Duration) TokenBuilder
	WithExpiresInCtx(ctx context.Context, expiresIn time.Duration) TokenBuilder
	WithSuperAdmin() TokenBuilder
	WithAdmin() TokenBuilder
	WithSelfSigned() TokenBuilder
	WithActorEmail(email string) TokenBuilder
	WithActorId(id string) TokenBuilder
	WithActor(actor core.IActorData) TokenBuilder
	WithNonce() TokenBuilder

	WithConfigKey(ctx context.Context, cfgKey *config.Key) (TokenBuilder, error)
	MustWithConfigKey(ctx context.Context, cfgKey *config.Key) TokenBuilder
	WithSecretConfigKeyData(ctx context.Context, cfgKeyData config.KeyDataType) (TokenBuilder, error)
	WithPrivateKeyPath(string) TokenBuilder
	WithPrivateKeyString(string) TokenBuilder
	WithPrivateKey([]byte) TokenBuilder
	WithSecretKeyPath(string) TokenBuilder
	WithSecretKeyString(string) TokenBuilder
	WithSecretKey([]byte) TokenBuilder

	TokenCtx(context.Context) (string, error)
	Token() (string, error)
	MustTokenCtx(context.Context) string
	MustToken() string

	Signer() (Signer, error)
	SignerCtx(context.Context) (Signer, error)
	MustSigner() Signer
	MustSignerCtx(context.Context) Signer
}

type tokenBuilder struct {
	jwtBuilder     claimsBuilder
	claims         *AuthProxyClaims
	privateKeyPath *string
	privateKeyData []byte
	secretKeyPath  *string
	secretKeyData  []byte
}

func (tb *tokenBuilder) WithClaims(c *AuthProxyClaims) TokenBuilder {
	tb.claims = c
	return tb
}

func (tb *tokenBuilder) WithIssuer(issuer string) TokenBuilder {
	tb.jwtBuilder.WithIssuer(issuer)
	return tb
}

func (tb *tokenBuilder) WithAudience(audience string) TokenBuilder {
	tb.jwtBuilder.WithAudience(audience)
	return tb
}

func (tb *tokenBuilder) WithServiceId(serviceId config.ServiceId) TokenBuilder {
	tb.jwtBuilder.WithServiceId(serviceId)
	return tb
}

func (tb *tokenBuilder) WithServiceIds(serviceIds []config.ServiceId) TokenBuilder {
	tb.jwtBuilder.WithServiceIds(serviceIds)
	return tb
}

func (tb *tokenBuilder) WithExpiration(expiration time.Time) TokenBuilder {
	tb.jwtBuilder.WithExpiration(expiration)
	return tb
}

func (tb *tokenBuilder) WithExpiresIn(expiresIn time.Duration) TokenBuilder {
	tb.jwtBuilder.WithExpiresIn(expiresIn)
	return tb
}

func (tb *tokenBuilder) WithExpiresInCtx(ctx context.Context, expiresIn time.Duration) TokenBuilder {
	tb.jwtBuilder.WithExpiresInCtx(ctx, expiresIn)
	return tb
}

func (tb *tokenBuilder) WithSuperAdmin() TokenBuilder {
	tb.jwtBuilder.WithSuperAdmin()
	return tb
}

func (tb *tokenBuilder) WithAdmin() TokenBuilder {
	tb.jwtBuilder.WithAdmin()
	return tb
}

func (tb *tokenBuilder) WithSelfSigned() TokenBuilder {
	tb.jwtBuilder.WithSelfSigned()
	return tb
}

func (tb *tokenBuilder) WithActorEmail(email string) TokenBuilder {
	tb.jwtBuilder.WithActorEmail(email)
	return tb
}

func (tb *tokenBuilder) WithActorId(id string) TokenBuilder {
	tb.jwtBuilder.WithActorExternalId(id)
	return tb
}

func (tb *tokenBuilder) WithActor(actor core.IActorData) TokenBuilder {
	tb.jwtBuilder.WithActor(actor)
	return tb
}

func (tb *tokenBuilder) WithNonce() TokenBuilder {
	tb.jwtBuilder.WithNonce()
	return tb
}

func (tb *tokenBuilder) WithConfigKey(ctx context.Context, cfgKey *config.Key) (TokenBuilder, error) {
	var us TokenBuilder = tb

	if pp, ok := cfgKey.InnerVal.(*config.KeyPublicPrivate); ok {
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

	if ks, ok := cfgKey.InnerVal.(*config.KeyShared); ok {
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

func (tb *tokenBuilder) MustWithConfigKey(ctx context.Context, cfgKey *config.Key) TokenBuilder {
	return util.Must(tb.WithConfigKey(ctx, cfgKey))
}

func (tb *tokenBuilder) WithSecretConfigKeyData(ctx context.Context, cfgKeyData config.KeyDataType) (TokenBuilder, error) {
	var us TokenBuilder = tb

	if cfgKeyData.HasData(ctx) {
		data, err := cfgKeyData.GetData(ctx)
		if err != nil {
			return us, err
		}
		us = us.WithSecretKey(data)
	}

	return us, nil
}

func (tb *tokenBuilder) WithPrivateKeyPath(privateKeyPath string) TokenBuilder {
	tb.privateKeyPath = &privateKeyPath
	return tb
}

func (tb *tokenBuilder) WithPrivateKeyString(privateKey string) TokenBuilder {
	return tb.WithPrivateKey([]byte(privateKey))
}

func (tb *tokenBuilder) WithPrivateKey(privateKey []byte) TokenBuilder {
	tb.privateKeyData = privateKey
	return tb
}

func (tb *tokenBuilder) WithSecretKeyPath(secretKeyPath string) TokenBuilder {
	tb.secretKeyPath = &secretKeyPath
	return tb
}

func (tb *tokenBuilder) WithSecretKeyString(secretKey string) TokenBuilder {
	return tb.WithSecretKey([]byte(secretKey))
}

func (tb *tokenBuilder) WithSecretKey(secretKey []byte) TokenBuilder {
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

func (tb *tokenBuilder) getSigningKeyDataAndMethod() (interface{}, jwt.SigningMethod, error) {
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

func (tb *tokenBuilder) TokenCtx(ctx context.Context) (string, error) {
	var claims *AuthProxyClaims
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

func (tb *tokenBuilder) Token() (string, error) {
	return tb.TokenCtx(context.Background())
}

func (tb *tokenBuilder) MustTokenCtx(ctx context.Context) string {
	token, err := tb.TokenCtx(ctx)
	if err != nil {
		panic(err)
	}

	return token
}

func (tb *tokenBuilder) MustToken() string {
	return tb.MustTokenCtx(context.Background())
}

func (tb *tokenBuilder) Signer() (Signer, error) {
	return tb.SignerCtx(context.Background())
}

func (tb *tokenBuilder) SignerCtx(ctx context.Context) (Signer, error) {
	token, err := tb.TokenCtx(ctx)
	if err != nil {
		return nil, err
	}

	return NewSigner(token), nil
}

func (tb *tokenBuilder) MustSigner() Signer {
	s, err := tb.Signer()
	if err != nil {
		panic(err)
	}

	return s
}

func (tb *tokenBuilder) MustSignerCtx(ctx context.Context) Signer {
	s, err := tb.SignerCtx(ctx)
	if err != nil {
		panic(err)
	}

	return s
}

func NewJwtTokenBuilder() TokenBuilder {
	return &tokenBuilder{}
}
