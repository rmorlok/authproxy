package config

import (
	"context"
	"crypto/tls"
	"fmt"

	"golang.org/x/crypto/acme/autocert"
)

type TlsConfigLetsEncrypt struct {
	AcceptTos     bool           `json:"acceptTos" yaml:"acceptTos"`
	Email         string         `json:"email" yaml:"email"`
	HostWhitelist []string       `json:"hostWhitelist" yaml:"hostWhitelist"`
	RenewBefore   *HumanDuration `json:"renewBefore,omitempty" yaml:"renewBefore,omitempty"`
	CacheDir      string         `json:"cacheDir" yaml:"cacheDir"`
}

func (tle *TlsConfigLetsEncrypt) TlsConfig(ctx context.Context, s HttpServiceLike) (*tls.Config, error) {
	if tle == nil {
		return nil, nil
	}

	if !tle.AcceptTos {
		return nil, fmt.Errorf("must accept tos to use lets encrypt")
	}

	if len(tle.HostWhitelist) == 0 {
		return nil, fmt.Errorf("must specify host whitelist to use lets encrypt")
	}

	if tle.CacheDir == "" {
		return nil, fmt.Errorf("must specify cache dir to use lets encrypt")
	}

	certManager := autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Email:      tle.Email,
		HostPolicy: autocert.HostWhitelist(tle.HostWhitelist...),
		Cache:      autocert.DirCache("certs"), // Optional: store certs in memory instead
	}

	if tle.RenewBefore != nil {
		certManager.RenewBefore = tle.RenewBefore.Duration
	}

	return certManager.TLSConfig(), nil
}

var _ TlsConfig = (*TlsConfigLetsEncrypt)(nil)
