package client

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// signJWT creates a signed JWT token string for the given username.
func signJWT(privateKeyPath, username string) (string, error) {
	keyData, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return "", fmt.Errorf("failed to read private key: %w", err)
	}

	key, err := parsePrivateKey(keyData)
	if err != nil {
		return "", fmt.Errorf("failed to parse private key: %w", err)
	}

	method := signingMethodForKey(key)
	now := time.Now()

	claims := jwt.MapClaims{
		"sub":           username,
		"iss":           "terraform-provider-authproxy",
		"aud":           []string{"admin-api"},
		"iat":           now.Unix(),
		"exp":           now.Add(1 * time.Hour).Unix(),
		"namespace":     "root",
		"actor_signed":  true,
		"system_signed": false,
	}

	token := jwt.NewWithClaims(method, claims)
	return token.SignedString(key)
}

func parsePrivateKey(data []byte) (crypto.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in private key")
	}

	// Try PKCS8 first (most common modern format)
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	// Try RSA PKCS1
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	// Try EC
	if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	return nil, fmt.Errorf("unsupported private key format")
}

func signingMethodForKey(key crypto.PrivateKey) jwt.SigningMethod {
	switch key.(type) {
	case *rsa.PrivateKey:
		return jwt.SigningMethodRS256
	case *ecdsa.PrivateKey:
		return jwt.SigningMethodES256
	case ed25519.PrivateKey:
		return jwt.SigningMethodEdDSA
	default:
		return jwt.SigningMethodRS256
	}
}
