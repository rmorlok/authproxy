package config

import (
	"context"
	"fmt"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

// KeyDataAwsSecret retrieves an AES key from AWS Secrets Manager.
type KeyDataAwsSecret struct {
	AwsSecretID  string          `json:"aws_secret_id" yaml:"aws_secret_id"`
	AwsSecretKey string          `json:"aws_secret_key,omitempty" yaml:"aws_secret_key,omitempty"`
	AwsRegion    string          `json:"aws_region,omitempty" yaml:"aws_region,omitempty"`
	Credentials  *AwsCredentials `json:"aws_credentials,omitempty" yaml:"aws_credentials,omitempty"`
	CacheTTL     string          `json:"cache_ttl,omitempty" yaml:"cache_ttl,omitempty"`

	cache cachedKeyFetcher
}

func (ka *KeyDataAwsSecret) GetCurrentVersion(ctx context.Context) (KeyVersionInfo, error) {
	ka.cache.fetch = func() ([]byte, error) {
		return ka.fetchFromAWS(ctx)
	}

	if ka.CacheTTL != "" {
		ttl, err := time.ParseDuration(ka.CacheTTL)
		if err != nil {
			return KeyVersionInfo{}, fmt.Errorf("invalid cache_ttl for aws secret key data: %w", err)
		}
		ka.cache.ttl = ttl
	}

	data, err := ka.cache.get()
	if err != nil {
		return KeyVersionInfo{}, err
	}

	providerID := ka.AwsSecretID
	if ka.AwsSecretKey != "" {
		providerID = ka.AwsSecretID + "/" + ka.AwsSecretKey
	}

	return KeyVersionInfo{
		Provider:        ProviderTypeAws,
		ProviderID:      providerID,
		ProviderVersion: DataHash(data),
		Data:            data,
		IsCurrent:       true,
	}, nil
}

func (ka *KeyDataAwsSecret) GetVersion(ctx context.Context, version string) (KeyVersionInfo, error) {
	data, err := ka.fetchFromAWSWithVersion(ctx, &version)
	if err != nil {
		return KeyVersionInfo{}, err
	}

	providerID := ka.AwsSecretID
	if ka.AwsSecretKey != "" {
		providerID = ka.AwsSecretID + "/" + ka.AwsSecretKey
	}

	current, _ := ka.GetCurrentVersion(ctx)
	isCurrent := current.ProviderVersion == DataHash(data)

	return KeyVersionInfo{
		Provider:        ProviderTypeAws,
		ProviderID:      providerID,
		ProviderVersion: version,
		Data:            data,
		IsCurrent:       isCurrent,
	}, nil
}

func (ka *KeyDataAwsSecret) ListVersions(ctx context.Context) ([]KeyVersionInfo, error) {
	v, err := ka.GetCurrentVersion(ctx)
	if err != nil {
		return nil, err
	}
	return []KeyVersionInfo{v}, nil
}

func (ka *KeyDataAwsSecret) GetProviderType() ProviderType {
	return ProviderTypeAws
}

func (ka *KeyDataAwsSecret) fetchFromAWS(ctx context.Context) ([]byte, error) {
	return ka.fetchFromAWSWithVersion(ctx, nil)
}

func (ka *KeyDataAwsSecret) fetchFromAWSWithVersion(ctx context.Context, versionID *string) ([]byte, error) {
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

	client := secretsmanager.NewFromConfig(cfg)

	input := &secretsmanager.GetSecretValueInput{
		SecretId: &ka.AwsSecretID,
	}
	if versionID != nil {
		input.VersionId = versionID
	}

	result, err := client.GetSecretValue(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret %s from AWS Secrets Manager: %w", ka.AwsSecretID, err)
	}

	if result.SecretString == nil {
		return nil, fmt.Errorf("secret %s has no string value", ka.AwsSecretID)
	}

	// If AwsSecretKey is specified, parse as JSON and extract the key
	if ka.AwsSecretKey != "" {
		return extractJsonKey(*result.SecretString, ka.AwsSecretKey)
	}

	return []byte(*result.SecretString), nil
}

var _ KeyDataType = (*KeyDataAwsSecret)(nil)
