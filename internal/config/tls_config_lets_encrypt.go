package config

import (
	"context"
	"crypto/tls"
	"fmt"
	"golang.org/x/crypto/acme/autocert"
)

type TlsConfigLetsEncrypt struct {
	AcceptTos     bool           `json:"accept_tos" yaml:"accept_tos"`
	Email         string         `json:"email" yaml:"email"`
	HostWhitelist []string       `json:"host_whitelist" yaml:"host_whitelist"`
	RenewBefore   *HumanDuration `json:"renew_before,omitempty" yaml:"renew_before,omitempty"`
	CacheDir      string         `json:"cache_dir" yaml:"cache_dir"`
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
