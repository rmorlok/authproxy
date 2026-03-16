package config

import (
	"context"
	"fmt"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	smtypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
)

// awsSecretsManagerClient is the subset of the AWS Secrets Manager API used by KeyDataAwsSecret.
type awsSecretsManagerClient interface {
	GetSecretValue(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
	ListSecretVersionIds(ctx context.Context, params *secretsmanager.ListSecretVersionIdsInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error)
}

// KeyDataAwsSecret retrieves an AES key from AWS Secrets Manager.
type KeyDataAwsSecret struct {
	AwsSecretID  string          `json:"aws_secret_id" yaml:"aws_secret_id"`
	AwsSecretKey string          `json:"aws_secret_key,omitempty" yaml:"aws_secret_key,omitempty"`
	AwsRegion    string          `json:"aws_region,omitempty" yaml:"aws_region,omitempty"`
	Credentials  *AwsCredentials `json:"aws_credentials,omitempty" yaml:"aws_credentials,omitempty"`
	CacheTTL     string          `json:"cache_ttl,omitempty" yaml:"cache_ttl,omitempty"`

	cache keyDataCache

	// clientFactory overrides the default AWS client creation for testing.
	clientFactory func(ctx context.Context) (awsSecretsManagerClient, error)
}

func (ka *KeyDataAwsSecret) initCache() error {
	if ka.cache.fetchCurrent != nil {
		return nil
	}

	if ka.CacheTTL != "" {
		ttl, err := time.ParseDuration(ka.CacheTTL)
		if err != nil {
			return fmt.Errorf("invalid cache_ttl for aws secret key data: %w", err)
		}
		ka.cache.ttl = ttl
	}

	ka.cache.fetchCurrent = ka.fetchCurrentVersion
	ka.cache.fetchVersion = ka.fetchVersion
	ka.cache.fetchList = ka.fetchListVersions
	return nil
}

func (ka *KeyDataAwsSecret) providerID() string {
	providerID := ka.AwsSecretID
	if ka.AwsSecretKey != "" {
		providerID = ka.AwsSecretID + "/" + ka.AwsSecretKey
	}
	return providerID
}

func (ka *KeyDataAwsSecret) fetchCurrentVersion(ctx context.Context) (KeyVersionInfo, error) {
	data, version, err := ka.fetchFromAWSWithVersion(ctx, nil)
	if err != nil {
		return KeyVersionInfo{}, err
	}

	return KeyVersionInfo{
		Provider:        ProviderTypeAws,
		ProviderID:      ka.providerID(),
		ProviderVersion: version,
		Data:            data,
		IsCurrent:       true,
	}, nil
}

func (ka *KeyDataAwsSecret) fetchVersion(ctx context.Context, version string) (KeyVersionInfo, error) {
	data, _, err := ka.fetchFromAWSWithVersion(ctx, &version)
	if err != nil {
		return KeyVersionInfo{}, err
	}

	return KeyVersionInfo{
		Provider:        ProviderTypeAws,
		ProviderID:      ka.providerID(),
		ProviderVersion: version,
		Data:            data,
	}, nil
}

func (ka *KeyDataAwsSecret) fetchListVersions(ctx context.Context) ([]KeyVersionInfo, error) {
	client, err := ka.getClient(ctx)
	if err != nil {
		return nil, err
	}

	// List all version IDs for this secret
	var versions []smtypes.SecretVersionsListEntry
	var nextToken *string
	for {
		out, err := client.ListSecretVersionIds(ctx, &secretsmanager.ListSecretVersionIdsInput{
			SecretId:  &ka.AwsSecretID,
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list versions for secret %s: %w", ka.AwsSecretID, err)
		}
		versions = append(versions, out.Versions...)
		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}

	// Determine which version ID is the current one (has AWSCURRENT staging label)
	var currentVersionID string
	for _, v := range versions {
		for _, stage := range v.VersionStages {
			if stage == "AWSCURRENT" {
				if v.VersionId != nil {
					currentVersionID = *v.VersionId
				}
				break
			}
		}
	}

	// Fetch each version individually
	var infos []KeyVersionInfo
	for _, v := range versions {
		if v.VersionId == nil {
			continue
		}
		vid := *v.VersionId
		info, err := ka.fetchVersion(ctx, vid)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch version %s for secret %s: %w", vid, ka.AwsSecretID, err)
		}
		info.IsCurrent = vid == currentVersionID
		infos = append(infos, info)
	}

	return infos, nil
}

func (ka *KeyDataAwsSecret) GetCurrentVersion(ctx context.Context) (KeyVersionInfo, error) {
	if err := ka.initCache(); err != nil {
		return KeyVersionInfo{}, err
	}
	return ka.cache.GetCurrentVersion(ctx)
}

func (ka *KeyDataAwsSecret) GetVersion(ctx context.Context, version string) (KeyVersionInfo, error) {
	if err := ka.initCache(); err != nil {
		return KeyVersionInfo{}, err
	}

	info, err := ka.cache.GetVersion(ctx, version)
	if err != nil {
		return KeyVersionInfo{}, err
	}

	current, _ := ka.cache.GetCurrentVersion(ctx)
	info.IsCurrent = current.ProviderVersion == version
	return info, nil
}

func (ka *KeyDataAwsSecret) ListVersions(ctx context.Context) ([]KeyVersionInfo, error) {
	if err := ka.initCache(); err != nil {
		return nil, err
	}
	return ka.cache.ListVersions(ctx)
}

func (ka *KeyDataAwsSecret) GetProviderType() ProviderType {
	return ProviderTypeAws
}

func (ka *KeyDataAwsSecret) getClient(ctx context.Context) (awsSecretsManagerClient, error) {
	if ka.clientFactory != nil {
		return ka.clientFactory(ctx)
	}
	return ka.newSMClient(ctx)
}

func (ka *KeyDataAwsSecret) newSMClient(ctx context.Context) (*secretsmanager.Client, error) {
	opts := []func(*awsconfig.LoadOptions) error{}

	if ka.AwsRegion != "" {
		opts = append(opts, awsconfig.WithRegion(ka.AwsRegion))
	}

	if ka.Credentials != nil {
		credOpts, err := ka.Credentials.GetAwsConfigLoadOptions(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get aws credentials: %w", err)
		}
		opts = append(opts, credOpts...)
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load aws config: %w", err)
	}

	return secretsmanager.NewFromConfig(cfg), nil
}

func (ka *KeyDataAwsSecret) fetchFromAWSWithVersion(ctx context.Context, versionID *string) ([]byte, string, error) {
	client, err := ka.getClient(ctx)
	if err != nil {
		return nil, "", err
	}

	input := &secretsmanager.GetSecretValueInput{
		SecretId: &ka.AwsSecretID,
	}
	if versionID != nil {
		input.VersionId = versionID
	}

	result, err := client.GetSecretValue(ctx, input)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get secret %s from AWS Secrets Manager: %w", ka.AwsSecretID, err)
	}

	var awsVersion string
	if result.VersionId != nil {
		awsVersion = *result.VersionId
	}

	var data []byte

	// If AwsSecretKey is specified, parse as JSON and extract the key
	if ka.AwsSecretKey != "" {
		if result.SecretString == nil {
			return nil, "", fmt.Errorf("secret %s has no string value", ka.AwsSecretID)
		}

		data, err = extractJsonKey(*result.SecretString, ka.AwsSecretKey)
		return data, awsVersion, err
	} else {
		if result.SecretString == nil && result.SecretBinary == nil {
			return nil, "", fmt.Errorf("secret %s has no string or binary value", ka.AwsSecretID)
		}

		if result.SecretBinary != nil {
			data = result.SecretBinary
		} else {
			data = []byte(*result.SecretString)
		}
	}

	return data, awsVersion, nil
}

var _ KeyDataType = (*KeyDataAwsSecret)(nil)
