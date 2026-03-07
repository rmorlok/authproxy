package config

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
)

type TlsConfigVals struct {
	Cert *KeyData `json:"cert" yaml:"cert"`
	Key  *KeyData `json:"key" yaml:"key"`
}

func (tcv *TlsConfigVals) TlsConfig(ctx context.Context, s HttpServiceLike) (*tls.Config, error) {
	if tcv == nil {
		return nil, nil
	}

	if tcv.Cert == nil || tcv.Key == nil {
		return nil, fmt.Errorf("tls config vals must have cert and key")
	}

	certVersion, err := tcv.Cert.GetCurrentVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("tls cert key data not available: %w", err)
	}

	keyVersion, err := tcv.Key.GetCurrentVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("tls key data not available: %w", err)
	}

	cert := certVersion.Data
	key := keyVersion.Data

	// Create certificate from byte slices
	kp, err := tls.X509KeyPair(cert, key)
	if err != nil {
		log.Fatalf("Failed to load key pair: %v", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{kp},
	}, nil
}

var _ TlsConfig = (*TlsConfigVals)(nil)
