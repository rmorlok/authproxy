package jwt

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"golang.org/x/crypto/ssh"
)

// KeySelector is a function that takes a claims object and loads a key dynamically used to verify the JWT. This
// selector can mean that different actors can be verified with different keys.
//
// Parameters:
// * ctx: Context used to load the key data
// * unverified: The claims that have been loaded from the JWT that have not yet had their signature verified
//
// Returns:
// * kd: the key data to use
// * isShared: if the key data is a shared (aka secret) key. If false, will assume public key.
// * err: An error from loading key data. If specified, other return values are ignored.
type KeySelector func(ctx context.Context, unverified *AuthProxyClaims) (kd config.KeyDataType, isShared bool, err error)

// ParserBuilder is a builder that can parse a JWT
type ParserBuilder interface {

	// WithKeySelector specifies a key selector function to dynamically load a key based on the unverified, parsed
	// JWT. This is useful for cases where the key used can vary based on the token issued.
	WithKeySelector(KeySelector) ParserBuilder

	// WithConfigKey specifies the key to be used for parsing as a config value. Key can be either secret or public.
	WithConfigKey(ctx context.Context, cfgKey *config.Key) ParserBuilder

	// WithPublicKeyPath specifies the public key as a file path.
	WithPublicKeyPath(string) ParserBuilder

	// WithPublicKeyString specifies the public key as an explicit string value.
	WithPublicKeyString(string) ParserBuilder

	// WithPublicKey sets the public key using the provided byte slice.
	WithPublicKey([]byte) ParserBuilder

	// WithSharedKeyPath sets the shared (aka secret) key using the file path provided.
	WithSharedKeyPath(string) ParserBuilder

	// WithSharedKeyString sets the shared (aka secret) key for the parser using a string.
	WithSharedKeyString(string) ParserBuilder

	// WithSharedKey sets the shared (aka secret) key for the JWT parser using the provided byte slice.
	WithSharedKey([]byte) ParserBuilder

	ParseCtx(context.Context, string) (*AuthProxyClaims, error)
	Parse(string) (*AuthProxyClaims, error)
	MustParseCtx(context.Context, string) AuthProxyClaims
	MustParse(string) AuthProxyClaims
}

type parserBuilder struct {
	keySelector   KeySelector
	key           *config.Key
	publicKeyPath *string
	publicKeyData []byte
	secretKeyPath *string
	secretKeyData []byte
}

func (pb *parserBuilder) WithKeySelector(
	selector KeySelector,
) ParserBuilder {
	pb.keySelector = selector
	return pb
}

func (pb *parserBuilder) defaultKeySelector(
	ctx context.Context,
	unverified *AuthProxyClaims,
) (config.KeyDataType, bool, error) {
	const (
		isPublicKey = false
		isSharedKey = true
	)

	if pb.key != nil {
		if pk, ok := pb.key.InnerVal.(*config.KeyPublicPrivate); ok {
			return pk.PublicKey, isPublicKey, nil
		}

		if sk, ok := pb.key.InnerVal.(*config.KeyShared); ok {
			return sk.SharedKey, isSharedKey, nil
		}
		return nil, isSharedKey, errors.New("invalid key type")
	}

	if pb.publicKeyData != nil && pb.publicKeyPath != nil {
		return nil, isSharedKey, errors.New("cannot specify secret key data and path")
	}

	if pb.secretKeyPath != nil && pb.secretKeyData != nil {
		return nil, isSharedKey, errors.New("cannot specify secret key data and path")
	}

	if pb.publicKeyData == nil && pb.publicKeyPath == nil &&
		pb.secretKeyPath == nil && pb.secretKeyData == nil {
		return nil, isSharedKey, errors.New("key material must be specified in some form")
	}

	if pb.publicKeyPath != nil {
		return &config.KeyDataFile{Path: *pb.publicKeyPath}, isPublicKey, nil
	}

	if pb.publicKeyData != nil {
		return &config.KeyDataRawVal{Raw: pb.publicKeyData}, isPublicKey, nil
	}

	if pb.secretKeyPath != nil {
		return &config.KeyDataFile{Path: *pb.secretKeyPath}, isSharedKey, nil
	}

	if pb.secretKeyData != nil {
		return &config.KeyDataRawVal{Raw: pb.secretKeyData}, isSharedKey, nil
	}

	return nil, isSharedKey, errors.New("no key specified")
}

func (pb *parserBuilder) WithConfigKey(ctx context.Context, cfgKey *config.Key) ParserBuilder {
	pb.key = cfgKey
	return pb
}

func (pb *parserBuilder) WithPublicKeyPath(publicKeyPath string) ParserBuilder {
	pb.publicKeyPath = &publicKeyPath
	return pb
}

func (pb *parserBuilder) WithPublicKeyString(publicKey string) ParserBuilder {
	return pb.WithPublicKey([]byte(publicKey))
}

func (pb *parserBuilder) WithPublicKey(publicKey []byte) ParserBuilder {
	pb.publicKeyData = publicKey
	return pb
}

func (pb *parserBuilder) WithSharedKeyPath(secretKeyPath string) ParserBuilder {
	pb.secretKeyPath = &secretKeyPath
	return pb
}

func (pb *parserBuilder) WithSharedKeyString(secretKey string) ParserBuilder {
	return pb.WithSharedKey([]byte(secretKey))
}

func (pb *parserBuilder) WithSharedKey(secretKey []byte) ParserBuilder {
	pb.secretKeyData = secretKey
	return pb
}

// loadPublicKeyFromPEMOrOpenSSH loads an RSA public key from a PEM file data
func loadPublicKeyFromPEMOrOpenSSH(keyData []byte) (interface{}, jwt.SigningMethod, error) {
	// Just the straight public key data
	parsedKey, err := ssh.ParsePublicKey(keyData)
	if err == nil {
		return signingKeyMethodFromParsedPublicKey(parsedKey)
	}

	// Allowed ssh keys format; e.g. ssh-rsa <base64> bobdole@example.com
	parsedKey, _, _, _, err = ssh.ParseAuthorizedKey(keyData)
	if err == nil {
		return signingKeyMethodFromParsedPublicKey(parsedKey)
	}

	// Decode the PEM block
	block, rest := pem.Decode(keyData)
	if block == nil {
		return nil, nil, fmt.Errorf("failed to decode key as OpenSSH RSA and failed to decode PEM block containing public key")
	}

	if block.Type == "EC PARAMETERS" {
		block, _ = pem.Decode(rest)
		if block == nil {
			return nil, nil, fmt.Errorf("EC PEM file contained EC PARMETERS but not EC PUBLIC KEY")
		}
	}

	switch block.Type {
	case "RSA PUBLIC KEY":
		// Parse the DER-encoded RSA public key
		publicKey, err := x509.ParsePKCS1PublicKey(block.Bytes)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "failed to parse RSA public key")
		}

		return publicKey, jwt.SigningMethodRS256, nil
	case "EC PUBLIC KEY":
		// Parse the EC public key
		publicKey, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "failed to parse EC public key")
		}

		return signingKeyMethodFromParsedPublicKey(publicKey)
	case "PUBLIC KEY":
		// Parse an unencrypted public key (PKCS#8 encoded)
		publicKey, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "failed to parse public key")
		}

		return signingKeyMethodFromParsedPublicKey(publicKey)
	default:
		return nil, nil, fmt.Errorf("unsupported public key type: %s", block.Type)
	}
}

func signingKeyMethodFromParsedPublicKey(parsedKey interface{}) (interface{}, jwt.SigningMethod, error) {
	switch k := parsedKey.(type) {
	case *rsa.PublicKey:
		return parsedKey, jwt.SigningMethodRS256, nil
	case *ecdsa.PublicKey:
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
	case *ed25519.PublicKey:
		return parsedKey, jwt.SigningMethodEdDSA, nil
	case ed25519.PublicKey:
		return parsedKey, jwt.SigningMethodEdDSA, nil
	case ssh.PublicKey:
		cert, ok := k.(ssh.CryptoPublicKey)
		if !ok {
			return nil, nil, errors.New("public key does not support conversion to crypto public key")
		}
		ret := cert.CryptoPublicKey()
		switch k.Type() {
		case "ssh-rsa":
			return ret, jwt.SigningMethodRS256, nil
		case "ecdsa-sha2-nistp256":
			return ret, jwt.SigningMethodES256, nil
		case "ecdsa-sha2-nistp384":
			return ret, jwt.SigningMethodES384, nil
		case "ecdsa-sha2-nistp521":
			return ret, jwt.SigningMethodES512, nil
		case "ssh-ed25519":
			return ret, jwt.SigningMethodEdDSA, nil
		default:
			return nil, nil, fmt.Errorf("unsupported ssh public key type: %s", k.Type())
		}
	default:
		return nil, nil, errors.New("unsupported public key type")
	}
}

func (pb *parserBuilder) getVerifyingKeyData(ctx context.Context, unverified *AuthProxyClaims) (interface{}, jwt.SigningMethod, error) {
	keySelector := pb.defaultKeySelector
	if pb.keySelector != nil {
		keySelector = pb.keySelector
	}

	keyData, isShared, err := keySelector(ctx, unverified)
	if err != nil {
		return nil, nil, err
	}

	if !keyData.HasData(ctx) {
		return nil, nil, errors.New("key data not available")
	}

	rawKeyData, err := keyData.GetData(ctx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to get key data")
	}

	if isShared {
		return rawKeyData, &jwt.SigningMethodHMAC{}, nil
	}

	return loadPublicKeyFromPEMOrOpenSSH(rawKeyData)
}

func (pb *parserBuilder) ParseCtx(ctx context.Context, token string) (*AuthProxyClaims, error) {
	if pb.secretKeyPath == nil && pb.secretKeyData == nil &&
		pb.publicKeyData == nil && pb.publicKeyPath == nil &&
		pb.keySelector == nil {
		return nil, errors.New("key material must be specified in some form")
	}

	parser := jwt.NewParser(
		jwt.WithTimeFunc(func() time.Time {
			return apctx.GetClock(ctx).Now()
		}),
	)

	// Now parse with verification, using the key for this actor
	parsed, err := parser.ParseWithClaims(token, &AuthProxyClaims{}, func(unverified *jwt.Token) (interface{}, error) {
		unverifiedClaims, ok := unverified.Claims.(*AuthProxyClaims)
		if !ok {
			return nil, errors.New("invalid token")
		}

		key, _, err := pb.getVerifyingKeyData(ctx, unverifiedClaims)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get key for token")
		}

		return key, nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "can't parse token")
	}

	claims, ok := parsed.Claims.(*AuthProxyClaims)
	if !ok {
		return nil, errors.New("invalid token")
	}

	return claims, nil
}

func (pb *parserBuilder) Parse(token string) (*AuthProxyClaims, error) {
	return pb.ParseCtx(context.Background(), token)
}

func (pb *parserBuilder) MustParseCtx(ctx context.Context, token string) AuthProxyClaims {
	claims, err := pb.ParseCtx(ctx, token)
	if err != nil {
		panic(err)
	}

	return *claims
}

func (pb *parserBuilder) MustParse(token string) AuthProxyClaims {
	return pb.MustParseCtx(context.Background(), token)
}

func NewJwtTokenParserBuilder() ParserBuilder {
	return &parserBuilder{}
}
