package config

import (
	"context"
	"fmt"
	"strings"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	secretmanagerpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"google.golang.org/api/iterator"
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
	data, resolvedVersion, err := kg.fetchFromGCP(ctx)
	if err != nil {
		return KeyVersionInfo{}, err
	}

	return KeyVersionInfo{
		Provider:        ProviderTypeGcp,
		ProviderID:      kg.secretResourceName(),
		ProviderVersion: resolvedVersion,
		Data:            data,
		IsCurrent:       true,
	}, nil
}

func (kg *KeyDataGcpSecret) fetchVersionInfo(ctx context.Context, version string) (KeyVersionInfo, error) {
	data, resolvedVersion, err := kg.fetchVersionFromGCP(ctx, version)
	if err != nil {
		return KeyVersionInfo{}, err
	}

	return KeyVersionInfo{
		Provider:        ProviderTypeGcp,
		ProviderID:      kg.secretResourceName(),
		ProviderVersion: resolvedVersion,
		Data:            data,
	}, nil
}

func (kg *KeyDataGcpSecret) fetchListVersions(ctx context.Context) ([]KeyVersionInfo, error) {
	// Determine the current version so we can tag it correctly. If this fails
	// it's not fatal — some listed versions still count as valid, they just
	// won't be flagged as current.
	currentInfo, currentErr := kg.fetchCurrentVersion(ctx)
	currentVersion := ""
	if currentErr == nil {
		currentVersion = currentInfo.ProviderVersion
	}

	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create gcp secret manager client: %w", err)
	}
	defer client.Close()

	it := client.ListSecretVersions(ctx, &secretmanagerpb.ListSecretVersionsRequest{
		Parent: kg.secretResourceName(),
	})

	var infos []KeyVersionInfo
	for {
		v, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to list gcp secret versions for %s: %w", kg.GcpSecretName, err)
		}

		// Skip versions that can't be accessed (disabled/destroyed).
		if v.State != secretmanagerpb.SecretVersion_ENABLED {
			continue
		}

		version := parseGcpSecretVersionFromName(v.Name)
		data, resolvedVersion, err := kg.fetchVersionFromGCP(ctx, version)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch version %s for secret %s: %w", version, kg.GcpSecretName, err)
		}

		infos = append(infos, KeyVersionInfo{
			Provider:        ProviderTypeGcp,
			ProviderID:      kg.secretResourceName(),
			ProviderVersion: resolvedVersion,
			Data:            data,
			IsCurrent:       currentVersion != "" && resolvedVersion == currentVersion,
		})
	}

	// If the iterator returned nothing (should be rare), fall back to the
	// current version so we don't wipe out the only known version.
	if len(infos) == 0 && currentErr == nil {
		return []KeyVersionInfo{currentInfo}, nil
	}

	return infos, nil
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

func (kg *KeyDataGcpSecret) fetchFromGCP(ctx context.Context) ([]byte, string, error) {
	return kg.fetchVersionFromGCP(ctx, "")
}

// fetchVersionFromGCP accesses the requested version of the secret and returns
// the payload along with the concrete version resolved by GCP (e.g., "5"
// rather than "latest") as reported by AccessSecretVersionResponse.Name.
func (kg *KeyDataGcpSecret) fetchVersionFromGCP(ctx context.Context, version string) ([]byte, string, error) {
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create gcp secret manager client: %w", err)
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
		return nil, "", fmt.Errorf("failed to access gcp secret %s version %s: %w", kg.GcpSecretName, version, err)
	}

	resolvedVersion := parseGcpSecretVersionFromName(result.Name)
	if resolvedVersion == "" {
		resolvedVersion = version
	}
	if resolvedVersion == "" {
		resolvedVersion = kg.currentVersionString()
	}

	return result.Payload.Data, resolvedVersion, nil
}

// parseGcpSecretVersionFromName extracts the version segment from a full
// Secret Manager version resource name like
// `projects/*/secrets/*/versions/5`.
func parseGcpSecretVersionFromName(name string) string {
	const marker = "/versions/"
	idx := strings.LastIndex(name, marker)
	if idx < 0 {
		return ""
	}
	return name[idx+len(marker):]
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
