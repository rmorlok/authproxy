package auth

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/context"
	"golang.org/x/crypto/ssh"
	"os"
	"time"
)

// JwtTokenParserBuilder is a builder that can parse a JWT
type JwtTokenParserBuilder interface {
	WithPublicKeyPath(string) JwtTokenParserBuilder
	WithPublicKeyString(string) JwtTokenParserBuilder
	WithPublicKey([]byte) JwtTokenParserBuilder
	WithSecretKeyPath(string) JwtTokenParserBuilder
	WithSecretKeyString(string) JwtTokenParserBuilder
	WithSecretKey([]byte) JwtTokenParserBuilder

	ParseCtx(context.Context, string) (*JwtTokenClaims, error)
	Parse(string) (*JwtTokenClaims, error)
	MustParseCtx(context.Context, string) JwtTokenClaims
	MustParse(string) JwtTokenClaims
}

type jwtTokenParserBuilder struct {
	jwtBuilder    jwtBuilder
	publicKeyPath *string
	publicKeyData []byte
	secretKeyPath *string
	secretKeyData []byte
}

func (pb *jwtTokenParserBuilder) WithPublicKeyPath(publicKeyPath string) JwtTokenParserBuilder {
	pb.publicKeyPath = &publicKeyPath
	return pb
}

func (pb *jwtTokenParserBuilder) WithPublicKeyString(publicKey string) JwtTokenParserBuilder {
	return pb.WithPublicKey([]byte(publicKey))
}

func (pb *jwtTokenParserBuilder) WithPublicKey(publicKey []byte) JwtTokenParserBuilder {
	pb.publicKeyData = publicKey
	return pb
}

func (pb *jwtTokenParserBuilder) WithSecretKeyPath(secretKeyPath string) JwtTokenParserBuilder {
	pb.secretKeyPath = &secretKeyPath
	return pb
}

func (pb *jwtTokenParserBuilder) WithSecretKeyString(secretKey string) JwtTokenParserBuilder {
	return pb.WithSecretKey([]byte(secretKey))
}

func (pb *jwtTokenParserBuilder) WithSecretKey(secretKey []byte) JwtTokenParserBuilder {
	pb.secretKeyData = secretKey
	return pb
}

func (pb *jwtTokenParserBuilder) getSigningMethod() jwt.SigningMethod {
	if pb.publicKeyData != nil || pb.publicKeyPath != nil {
		return jwt.SigningMethodRS256
	}

	return jwt.SigningMethodHS256
}

// loadRSAPublicKeyFromPEMOrOpenSSH loads an RSA public key from a PEM file data
func loadRSAPublicKeyFromPEMOrOpenSSH(keyData []byte) (*rsa.PublicKey, error) {
	// Just the straight public key data
	parsedKey, err := ssh.ParsePublicKey(keyData)
	if err == nil {
		cpk, ok := parsedKey.(ssh.CryptoPublicKey)
		if !ok {
			return nil, fmt.Errorf("not an RSA public key")
		}

		if rsaKey, ok := cpk.CryptoPublicKey().(*rsa.PublicKey); ok {
			return rsaKey, nil
		}

		return nil, fmt.Errorf("failed to assert PublicKey type")
	}

	// Allowed ssh keys format; e.g. ssh-rsa <base64> bobdole@example.com
	parsedKey, _, _, _, err = ssh.ParseAuthorizedKey(keyData)
	if err == nil {
		cpk, ok := parsedKey.(ssh.CryptoPublicKey)
		if !ok {
			return nil, fmt.Errorf("not an RSA public key")
		}

		if rsaKey, ok := cpk.CryptoPublicKey().(*rsa.PublicKey); ok {
			return rsaKey, nil
		}

		return nil, fmt.Errorf("failed to assert PublicKey type")
	}

	// Decode the PEM block
	block, _ := pem.Decode(keyData)
	if block == nil || block.Type != "PUBLIC KEY" {
		return nil, fmt.Errorf("failed to decode key as OpenSSH RSA and failed to decode PEM block containing public key")
	}

	// Parse the DER-encoded RSA public key
	publicKey, err := x509.ParsePKCS1PublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse RSA public key: %w", err)
	}

	return publicKey, nil
}

func (pb *jwtTokenParserBuilder) getVerifyingKeyData() (interface{}, error) {
	if pb.publicKeyData != nil && pb.publicKeyPath != nil {
		return nil, errors.New("cannot specify secret key data and path")
	}

	if pb.secretKeyPath != nil && pb.secretKeyData != nil {
		return nil, errors.New("cannot specify secret key data and path")
	}

	if pb.publicKeyData == nil && pb.publicKeyPath == nil &&
		pb.secretKeyPath == nil && pb.secretKeyData == nil {
		return nil, errors.New("key material must be specified in some form")
	}

	if pb.publicKeyData != nil {
		return loadRSAPublicKeyFromPEMOrOpenSSH(pb.publicKeyData)
	}

	if pb.secretKeyData != nil {
		return pb.secretKeyData, nil
	}

	pathType := "public"
	isPublic := true
	pathPtr := pb.publicKeyPath

	if pb.secretKeyPath != nil {
		pathType = "secret"
		isPublic = false
		pathPtr = pb.secretKeyPath
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

	if isPublic {
		return loadRSAPublicKeyFromPEMOrOpenSSH(keyData)
	}

	return keyData, nil
}

func (pb *jwtTokenParserBuilder) ParseCtx(ctx context.Context, token string) (*JwtTokenClaims, error) {
	if pb.secretKeyPath == nil && pb.secretKeyData == nil &&
		pb.publicKeyData == nil && pb.publicKeyPath == nil {
		return nil, errors.New("key material must be specified in some form")
	}

	parser := jwt.NewParser(
		jwt.WithTimeFunc(func() time.Time {
			return ctx.Clock().Now()
		}),
	)

	// Now parse with verification, using the key for this actor
	parsed, err := parser.ParseWithClaims(token, &JwtTokenClaims{}, func(unverified *jwt.Token) (interface{}, error) {
		switch unverified.Method.(type) {
		case *jwt.SigningMethodHMAC:
			if pb.secretKeyPath == nil && pb.secretKeyData == nil {
				return nil, errors.New("token is signed with secret key, but public key given")
			}
		case *jwt.SigningMethodRSA:
			if pb.publicKeyPath == nil && pb.publicKeyData == nil {
				return nil, errors.New("token is signed with public key, but secret key given")
			}
		default:
			return nil, errors.Errorf("unexpected signing method: %v", unverified.Header["alg"])
		}

		return pb.getVerifyingKeyData()
	})
	if err != nil {
		return nil, errors.Wrap(err, "can't parse token")
	}

	claims, ok := parsed.Claims.(*JwtTokenClaims)
	if !ok {
		return nil, errors.New("invalid token")
	}

	return claims, nil
}

func (pb *jwtTokenParserBuilder) Parse(token string) (*JwtTokenClaims, error) {
	return pb.ParseCtx(context.Background(), token)
}

func (pb *jwtTokenParserBuilder) MustParseCtx(ctx context.Context, token string) JwtTokenClaims {
	claims, err := pb.ParseCtx(ctx, token)
	if err != nil {
		panic(err)
	}

	return *claims
}

func (pb *jwtTokenParserBuilder) MustParse(token string) JwtTokenClaims {
	return pb.MustParseCtx(context.Background(), token)
}

func NewJwtTokenParserBuilder() JwtTokenParserBuilder {
	return &jwtTokenParserBuilder{}
}
