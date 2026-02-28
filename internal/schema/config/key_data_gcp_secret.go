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

	cache keyDataCache
}

func (kg *KeyDataGcpSecret) initCache() error {
	if kg.cache.fetchCurrent != nil {
		return nil
	}

	if kg.CacheTTL != "" {
		ttl, err := time.ParseDuration(kg.CacheTTL)
		if err != nil {
			return fmt.Errorf("invalid cache_ttl for gcp secret key data: %w", err)
		}
		kg.cache.ttl = ttl
	}

	kg.cache.fetchCurrent = kg.fetchCurrentVersion
	kg.cache.fetchVersion = kg.fetchVersionInfo
	kg.cache.fetchList = kg.fetchListVersions
	return nil
}

func (kg *KeyDataGcpSecret) currentVersionString() string {
	version := kg.GcpSecretVersion
	if version == "" {
		version = "latest"
	}
	return version
}

func (kg *KeyDataGcpSecret) fetchCurrentVersion(ctx context.Context) (KeyVersionInfo, error) {
	data, err := kg.fetchFromGCP(ctx)
	if err != nil {
		return KeyVersionInfo{}, err
	}

	return KeyVersionInfo{
		Provider:        ProviderTypeGcp,
		ProviderID:      kg.secretResourceName(),
		ProviderVersion: kg.currentVersionString(),
		Data:            data,
		IsCurrent:       true,
	}, nil
}

func (kg *KeyDataGcpSecret) fetchVersionInfo(ctx context.Context, version string) (KeyVersionInfo, error) {
	data, err := kg.fetchVersionFromGCP(ctx, version)
	if err != nil {
		return KeyVersionInfo{}, err
	}

	return KeyVersionInfo{
		Provider:        ProviderTypeGcp,
		ProviderID:      kg.secretResourceName(),
		ProviderVersion: version,
		Data:            data,
		IsCurrent:       version == kg.currentVersionString(),
	}, nil
}

func (kg *KeyDataGcpSecret) fetchListVersions(ctx context.Context) ([]KeyVersionInfo, error) {
	v, err := kg.fetchCurrentVersion(ctx)
	if err != nil {
		return nil, err
	}
	return []KeyVersionInfo{v}, nil
}

func (kg *KeyDataGcpSecret) GetCurrentVersion(ctx context.Context) (KeyVersionInfo, error) {
	if err := kg.initCache(); err != nil {
		return KeyVersionInfo{}, err
	}
	return kg.cache.GetCurrentVersion(ctx)
}

func (kg *KeyDataGcpSecret) GetVersion(ctx context.Context, version string) (KeyVersionInfo, error) {
	if err := kg.initCache(); err != nil {
		return KeyVersionInfo{}, err
	}
	return kg.cache.GetVersion(ctx, version)
}

func (kg *KeyDataGcpSecret) ListVersions(ctx context.Context) ([]KeyVersionInfo, error) {
	if err := kg.initCache(); err != nil {
		return nil, err
	}
	return kg.cache.ListVersions(ctx)
}

func (kg *KeyDataGcpSecret) GetProviderType() ProviderType {
	return ProviderTypeGcp
}

func (kg *KeyDataGcpSecret) secretResourceName() string {
	if len(kg.GcpSecretName) > 0 && kg.GcpSecretName[0] == 'p' {
		return kg.GcpSecretName
	}
	if kg.GcpProject == "" {
		return fmt.Sprintf("projects/-/secrets/%s", kg.GcpSecretName)
	}
	return fmt.Sprintf("projects/%s/secrets/%s", kg.GcpProject, kg.GcpSecretName)
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
	return kg.fetchVersionFromGCP(ctx, "")
}

func (kg *KeyDataGcpSecret) fetchVersionFromGCP(ctx context.Context, version string) ([]byte, error) {
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create gcp secret manager client: %w", err)
	}
	defer client.Close()

	versionName := kg.secretVersionName()
	if version != "" {
		versionName = kg.secretVersionNameFor(version)
	}

	result, err := client.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{
		Name: versionName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to access gcp secret %s version %s: %w", kg.GcpSecretName, version, err)
	}

	return result.Payload.Data, nil
}

func (kg *KeyDataGcpSecret) secretVersionNameFor(version string) string {
	if len(kg.GcpSecretName) > 0 && kg.GcpSecretName[0] == 'p' {
		return fmt.Sprintf("%s/versions/%s", kg.GcpSecretName, version)
	}

	if kg.GcpProject == "" {
		return fmt.Sprintf("projects/-/secrets/%s/versions/%s", kg.GcpSecretName, version)
	}

	return fmt.Sprintf("projects/%s/secrets/%s/versions/%s", kg.GcpProject, kg.GcpSecretName, version)
}

var _ KeyDataType = (*KeyDataGcpSecret)(nil)
