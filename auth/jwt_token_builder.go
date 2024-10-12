package auth

import (
	"github.com/golang-jwt/jwt/v5"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/context"
	"os"
	"time"
)

// JwtTokenBuilder extends from JwtBuilder to provide options to sign tokens
type JwtTokenBuilder interface {
	WithIssuer(issuer string) JwtTokenBuilder
	WithAudience(audience string) JwtTokenBuilder
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

	TokenCtx(context.Context) (string, error)
	Token() (string, error)
	MustTokenCtx(context.Context) string
	MustToken() string
}

type jwtTokenBuilder struct {
	jwtBuilder     jwtBuilder
	privateKeyPath *string
	privateKeyData []byte
}

func (tb *jwtTokenBuilder) WithIssuer(issuer string) JwtTokenBuilder {
	tb.jwtBuilder.WithIssuer(issuer)
	return tb
}

func (tb *jwtTokenBuilder) WithAudience(audience string) JwtTokenBuilder {
	tb.jwtBuilder.WithAudience(audience)
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

func (tb *jwtTokenBuilder) getPrivateKeyData() ([]byte, error) {
	if tb.privateKeyData != nil && tb.privateKeyPath != nil {
		return nil, errors.New("cannot specify private key data and path")
	}

	if tb.privateKeyData != nil {
		return tb.privateKeyData, nil
	}

	privateKeyPath := *tb.privateKeyPath
	_, err := os.Stat(privateKeyPath)
	if err != nil {
		// attempt home path expansion
		privateKeyPath, err = homedir.Expand(privateKeyPath)
		if err != nil {
			return nil, err
		}
	}

	_, err = os.Stat(privateKeyPath)
	if err != nil {
		return nil, errors.Wrap(err, "invalid private key path")
	}

	privateKeyData, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, errors.Wrap(err, "error reading private key")
	}

	return privateKeyData, nil
}

func (tb *jwtTokenBuilder) TokenCtx(ctx context.Context) (string, error) {
	claims, err := tb.jwtBuilder.BuildCtx(ctx)
	if err != nil {
		return "", err
	}

	privateKeyData, err := tb.getPrivateKeyData()
	if err != nil {
		return "", err
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(privateKeyData)
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
