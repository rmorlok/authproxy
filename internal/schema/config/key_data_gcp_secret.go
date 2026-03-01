package config

import (
	"context"
	"fmt"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	secretmanagerpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
)

// KeyDataGcpSecret retrieves an AES key from GCP Secret Manager.
type KeyDataGcpSecret struct {
	GcpSecretName    string `json:"gcp_secret_name" yaml:"gcp_secret_name"`
	GcpProject       string `json:"gcp_project,omitempty" yaml:"gcp_project,omitempty"`
	GcpSecretVersion string `json:"gcp_secret_version,omitempty" yaml:"gcp_secret_version,omitempty"`
	CacheTTL         string `json:"cache_ttl,omitempty" yaml:"cache_ttl,omitempty"`

	cache cachedKeyFetcher
}

func (kg *KeyDataGcpSecret) HasData(ctx context.Context) bool {
	return kg.GcpSecretName != ""
}

func (kg *KeyDataGcpSecret) GetData(ctx context.Context) ([]byte, error) {
	kg.cache.fetch = func() ([]byte, error) {
		return kg.fetchFromGCP(ctx)
	}

	if kg.CacheTTL != "" {
		ttl, err := time.ParseDuration(kg.CacheTTL)
		if err != nil {
			return nil, fmt.Errorf("invalid cache_ttl for gcp secret key data: %w", err)
		}
		kg.cache.ttl = ttl
	}

	return kg.cache.get()
}

func (kg *KeyDataGcpSecret) secretVersionName() string {
	version := kg.GcpSecretVersion
	if version == "" {
		version = "latest"
	}

	// If it already looks like a full resource name, use it directly
	if len(kg.GcpSecretName) > 0 && kg.GcpSecretName[0] == 'p' {
		// Full resource name: projects/*/secrets/*/versions/*
		return fmt.Sprintf("%s/versions/%s", kg.GcpSecretName, version)
	}

	if kg.GcpProject == "" {
		return fmt.Sprintf("projects/-/secrets/%s/versions/%s", kg.GcpSecretName, version)
	}

	return fmt.Sprintf("projects/%s/secrets/%s/versions/%s", kg.GcpProject, kg.GcpSecretName, version)
}

func (kg *KeyDataGcpSecret) fetchFromGCP(ctx context.Context) ([]byte, error) {
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create gcp secret manager client: %w", err)
	}
	defer client.Close()

	result, err := client.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{
		Name: kg.secretVersionName(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to access gcp secret %s: %w", kg.GcpSecretName, err)
	}

	return result.Payload.Data, nil
}

var _ KeyDataType = (*KeyDataGcpSecret)(nil)
