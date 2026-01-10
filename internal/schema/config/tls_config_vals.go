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

	if !tcv.Cert.HasData(ctx) || !tcv.Key.HasData(ctx) {
		return nil, fmt.Errorf("tls config vals must have cert and key data")
	}

	cert, err := tcv.Cert.GetData(ctx)
	if err != nil {
		return nil, err
	}

	key, err := tcv.Key.GetData(ctx)
	if err != nil {
		return nil, err
	}

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
