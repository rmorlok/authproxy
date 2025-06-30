package config

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"github.com/mitchellh/go-homedir"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

type TlsConfigSelfSignedAutogen struct {
	AutoGenPath string `json:"auto_gen_path" yaml:"auto_gen_path"`
}

const (
	autogenCertFileName = "cert.pem"
	autogenKeyFileName  = "key.pem"
)

func (a *TlsConfigSelfSignedAutogen) TlsConfig(ctx context.Context, s HttpService) (*tls.Config, error) {
	if a == nil {
		return nil, nil
	}

	if a.AutoGenPath == "" {
		return nil, fmt.Errorf("auto gen path must be specified")
	}

	path := a.AutoGenPath
	if _, err := os.Stat(path); err != nil {
		// attempt home path expansion
		path, err = homedir.Expand(path)
		if err != nil {
			return nil, err
		}
	}

	certPath := filepath.Join(path, autogenCertFileName)
	keyPath := filepath.Join(path, autogenKeyFileName)

	// Check if files already exist
	_, certErr := os.Stat(certPath)
	_, keyErr := os.Stat(keyPath)

	// Generate new certificate if either file is missing
	if os.IsNotExist(certErr) || os.IsNotExist(keyErr) {
		priv, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, fmt.Errorf("failed to generate private key: %v", err)
		}

		domain := "localhost"
		if s.Domain() != "" {
			domain = s.Domain()
		}

		var ip_addresses []net.IP
		if domain == "localhost" {
			ip_addresses = []net.IP{net.ParseIP("127.0.0.1")}
		}

		// Create a more complete certificate template
		template := x509.Certificate{
			SerialNumber: big.NewInt(1),
			Subject: pkix.Name{
				Organization: []string{"AuthProxy Development Certificate"},
				CommonName:   domain,
			},
			NotBefore:   time.Now(),
			NotAfter:    time.Now().Add(time.Hour * 24 * 365), // 1 year validity
			KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
			ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			// Add Subject Alternative Names for localhost and 127.0.0.1
			DNSNames:    []string{domain},
			IPAddresses: ip_addresses,
			// Make this a self-signed CA certificate
			IsCA:                  true,
			BasicConstraintsValid: true,
		}

		derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
		if err != nil {
			return nil, fmt.Errorf("failed to create certificate: %v", err)
		}

		// Create directory if it doesn't exist
		if err := os.MkdirAll(a.AutoGenPath, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory: %v", err)
		}

		// Save certificate
		certOut, err := os.Create(certPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open cert.pem for writing: %v", err)
		}
		if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
			certOut.Close()
			return nil, fmt.Errorf("failed to write cert.pem: %v", err)
		}
		certOut.Close()

		// Save private key
		keyOut, err := os.Create(keyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open key.pem for writing: %v", err)
		}
		privBytes := x509.MarshalPKCS1PrivateKey(priv)
		if err := pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: privBytes}); err != nil {
			keyOut.Close()
			return nil, fmt.Errorf("failed to write key.pem: %v", err)
		}
		keyOut.Close()

		fmt.Println("Generated new self-signed certificate for development. To avoid browser warnings:")
		fmt.Println("1. Import this certificate into your system trust store:")
		fmt.Printf("   %s\n", certPath)
		fmt.Println("2. Or use the -k flag with curl for testing:")
		fmt.Println(fmt.Sprintf("   curl -k %s/ping", s.GetBaseUrl()))
	}

	// Load certificate and key
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load key pair: %v", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
	}, nil

}

var _ TlsConfig = (*TlsConfigSelfSignedAutogen)(nil)
