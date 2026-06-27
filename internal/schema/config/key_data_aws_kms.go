package config

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmstypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
)

const (
	awsKMSProtectedDataType = string(ProviderTypeAwsKMS)

	awsKMSMetadataConfiguredKeyID = "aws_kms_configured_key_id"
	awsKMSMetadataKeyARN          = "aws_kms_key_arn"
	awsKMSMetadataKeyMaterialID   = "aws_kms_key_material_id"
)

// awsKMSClient is the subset of the AWS KMS API used by KeyDataAwsKMS.
type awsKMSClient interface {
	DescribeKey(ctx context.Context, params *kms.DescribeKeyInput, optFns ...func(*kms.Options)) (*kms.DescribeKeyOutput, error)
	GenerateDataKey(ctx context.Context, params *kms.GenerateDataKeyInput, optFns ...func(*kms.Options)) (*kms.GenerateDataKeyOutput, error)
	Encrypt(ctx context.Context, params *kms.EncryptInput, optFns ...func(*kms.Options)) (*kms.EncryptOutput, error)
	Decrypt(ctx context.Context, params *kms.DecryptInput, optFns ...func(*kms.Options)) (*kms.DecryptOutput, error)
}

// KeyDataAwsKMS uses AWS KMS as a wrapping-key provider for data encryption keys.
type KeyDataAwsKMS struct {
	AwsKMSKeyID    string          `json:"aws_kms_key_id" yaml:"aws_kms_key_id"`
	AwsRegion      string          `json:"aws_region,omitempty" yaml:"aws_region,omitempty"`
	AwsKMSEndpoint string          `json:"aws_kms_endpoint,omitempty" yaml:"aws_kms_endpoint,omitempty"`
	Credentials    *AwsCredentials `json:"aws_credentials,omitempty" yaml:"aws_credentials,omitempty"`
	CacheTTL       string          `json:"cache_ttl,omitempty" yaml:"cache_ttl,omitempty"`

	cache keyDataCache

	// clientFactory overrides the default AWS client creation for testing.
	clientFactory func(ctx context.Context) (awsKMSClient, error)
}

func (ka *KeyDataAwsKMS) initCache() error {
	if ka.cache.fetchCurrent != nil {
		return nil
	}

	if ka.CacheTTL != "" {
		ttl, err := time.ParseDuration(ka.CacheTTL)
		if err != nil {
			return fmt.Errorf("invalid cache_ttl for aws kms key data: %w", err)
		}
		ka.cache.ttl = ttl
	}

	ka.cache.fetchCurrent = ka.fetchCurrentVersion
	ka.cache.fetchVersion = ka.fetchVersion
	ka.cache.fetchList = ka.fetchListVersions
	return nil
}

func (ka *KeyDataAwsKMS) providerID() string {
	return ka.AwsKMSKeyID
}

func (ka *KeyDataAwsKMS) fetchCurrentVersion(ctx context.Context) (KeyVersionInfo, error) {
	current, err := ka.CurrentWrappingKey(ctx)
	if err != nil {
		return KeyVersionInfo{}, err
	}

	return KeyVersionInfo{
		Provider:        current.Provider,
		ProviderID:      current.ProviderID,
		ProviderVersion: current.ProviderVersion,
		IsCurrent:       true,
	}, nil
}

func (ka *KeyDataAwsKMS) fetchVersion(ctx context.Context, version string) (KeyVersionInfo, error) {
	current, err := ka.fetchCurrentVersion(ctx)
	if err != nil {
		return KeyVersionInfo{}, err
	}
	if current.ProviderVersion == version {
		return current, nil
	}

	return KeyVersionInfo{
		Provider:        ProviderTypeAwsKMS,
		ProviderID:      ka.providerID(),
		ProviderVersion: version,
	}, nil
}

func (ka *KeyDataAwsKMS) fetchListVersions(ctx context.Context) ([]KeyVersionInfo, error) {
	current, err := ka.fetchCurrentVersion(ctx)
	if err != nil {
		return nil, err
	}
	return []KeyVersionInfo{current}, nil
}

func (ka *KeyDataAwsKMS) GetCurrentVersion(ctx context.Context) (KeyVersionInfo, error) {
	if err := ka.initCache(); err != nil {
		return KeyVersionInfo{}, err
	}
	return ka.cache.GetCurrentVersion(ctx)
}

func (ka *KeyDataAwsKMS) GetVersion(ctx context.Context, version string) (KeyVersionInfo, error) {
	if err := ka.initCache(); err != nil {
		return KeyVersionInfo{}, err
	}
	return ka.cache.GetVersion(ctx, version)
}

func (ka *KeyDataAwsKMS) ListVersions(ctx context.Context) ([]KeyVersionInfo, error) {
	if err := ka.initCache(); err != nil {
		return nil, err
	}
	return ka.cache.ListVersions(ctx)
}

func (ka *KeyDataAwsKMS) ListVersionsWithDataEncryptionKeys(ctx context.Context, deks []DataEncryptionKeyInfo) ([]KeyVersionInfo, error) {
	var result []KeyVersionInfo
	for _, dekInfo := range deks {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if dekInfo.Provider != ProviderTypeAwsKMS {
			continue
		}

		dekBytes, err := ka.UnwrapDataEncryptionKey(ctx, dekInfo)
		if err != nil {
			return nil, err
		}

		result = append(result, KeyVersionInfo{
			Provider:        ProviderTypeAwsKMS,
			ProviderID:      dekInfo.ID,
			ProviderVersion: dekInfo.ProviderVersion,
			Data:            dekBytes,
			IsCurrent:       dekInfo.IsCurrent,
		})
	}

	return result, nil
}

func (ka *KeyDataAwsKMS) CurrentWrappingKey(ctx context.Context) (KeyWrappingKeyInfo, error) {
	if err := ctx.Err(); err != nil {
		return KeyWrappingKeyInfo{}, err
	}

	client, err := ka.getClient(ctx)
	if err != nil {
		return KeyWrappingKeyInfo{}, err
	}

	out, err := client.DescribeKey(ctx, &kms.DescribeKeyInput{
		KeyId: aws.String(ka.AwsKMSKeyID),
	})
	if err != nil {
		return KeyWrappingKeyInfo{}, fmt.Errorf("failed to describe AWS KMS key %s: %w", ka.AwsKMSKeyID, err)
	}
	if out.KeyMetadata == nil {
		return KeyWrappingKeyInfo{}, fmt.Errorf("AWS KMS describe key %s returned no metadata", ka.AwsKMSKeyID)
	}

	metadata := ka.providerMetadataFromKeyMetadata(out.KeyMetadata)
	version := awsKMSProviderVersionFromMetadata(metadata)
	if version == "" {
		return KeyWrappingKeyInfo{}, fmt.Errorf("AWS KMS key %s returned no usable provider version", ka.AwsKMSKeyID)
	}

	return KeyWrappingKeyInfo{
		Provider:        ProviderTypeAwsKMS,
		ProviderID:      ka.providerID(),
		ProviderVersion: version,
		Metadata:        metadata,
	}, nil
}

func (ka *KeyDataAwsKMS) GenerateDataEncryptionKey(ctx context.Context) (GeneratedDataEncryptionKey, error) {
	if err := ctx.Err(); err != nil {
		return GeneratedDataEncryptionKey{}, err
	}

	client, err := ka.getClient(ctx)
	if err != nil {
		return GeneratedDataEncryptionKey{}, err
	}

	out, err := client.GenerateDataKey(ctx, &kms.GenerateDataKeyInput{
		KeyId:   aws.String(ka.AwsKMSKeyID),
		KeySpec: kmstypes.DataKeySpecAes256,
	})
	if err != nil {
		return GeneratedDataEncryptionKey{}, fmt.Errorf("failed to generate data encryption key with AWS KMS key %s: %w", ka.AwsKMSKeyID, err)
	}
	if len(out.Plaintext) == 0 {
		return GeneratedDataEncryptionKey{}, errors.New("AWS KMS GenerateDataKey returned empty plaintext")
	}
	if len(out.CiphertextBlob) == 0 {
		return GeneratedDataEncryptionKey{}, errors.New("AWS KMS GenerateDataKey returned empty ciphertext")
	}

	metadata := ka.providerMetadataFromKMSResult(out.KeyId, out.KeyMaterialId)
	if metadata[awsKMSMetadataKeyARN] == "" || metadata[awsKMSMetadataKeyMaterialID] == "" {
		current, err := ka.CurrentWrappingKey(ctx)
		if err != nil {
			return GeneratedDataEncryptionKey{}, err
		}
		for k, v := range current.Metadata {
			if metadata[k] == "" {
				metadata[k] = v
			}
		}
	}
	version := awsKMSProviderVersionFromMetadata(metadata)
	protected := awsKMSProtectedData(out.CiphertextBlob, metadata)

	return GeneratedDataEncryptionKey{
		Provider:         ProviderTypeAwsKMS,
		ProviderID:       ka.providerID(),
		ProviderVersion:  version,
		ProviderMetadata: metadata,
		ProtectedData:    protected,
		Data:             append([]byte(nil), out.Plaintext...),
	}, nil
}

func (ka *KeyDataAwsKMS) WrapDataEncryptionKey(ctx context.Context, dek []byte) (GeneratedDataEncryptionKey, error) {
	if err := ctx.Err(); err != nil {
		return GeneratedDataEncryptionKey{}, err
	}
	if len(dek) == 0 {
		return GeneratedDataEncryptionKey{}, errors.New("data encryption key is empty")
	}

	current, err := ka.CurrentWrappingKey(ctx)
	if err != nil {
		return GeneratedDataEncryptionKey{}, err
	}

	client, err := ka.getClient(ctx)
	if err != nil {
		return GeneratedDataEncryptionKey{}, err
	}

	out, err := client.Encrypt(ctx, &kms.EncryptInput{
		KeyId:     aws.String(ka.AwsKMSKeyID),
		Plaintext: dek,
	})
	if err != nil {
		return GeneratedDataEncryptionKey{}, fmt.Errorf("failed to wrap data encryption key with AWS KMS key %s: %w", ka.AwsKMSKeyID, err)
	}
	if len(out.CiphertextBlob) == 0 {
		return GeneratedDataEncryptionKey{}, errors.New("AWS KMS Encrypt returned empty ciphertext")
	}

	metadata := copyStringMap(current.Metadata)
	if out.KeyId != nil && *out.KeyId != "" {
		metadata[awsKMSMetadataKeyARN] = *out.KeyId
	}

	return GeneratedDataEncryptionKey{
		Provider:         ProviderTypeAwsKMS,
		ProviderID:       current.ProviderID,
		ProviderVersion:  current.ProviderVersion,
		ProviderMetadata: metadata,
		ProtectedData:    awsKMSProtectedData(out.CiphertextBlob, metadata),
		Data:             append([]byte(nil), dek...),
	}, nil
}

func (ka *KeyDataAwsKMS) UnwrapDataEncryptionKey(ctx context.Context, dekInfo DataEncryptionKeyInfo) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if dekInfo.Provider != ProviderTypeAwsKMS {
		return nil, fmt.Errorf("unsupported AWS KMS provider %q", dekInfo.Provider)
	}
	if dekInfo.ProtectedData == nil || dekInfo.ProtectedData.IsZero() {
		return nil, errors.New("data encryption key protected data is empty")
	}
	if dekInfo.ProtectedData.Type != awsKMSProtectedDataType {
		return nil, fmt.Errorf("unsupported AWS KMS protected data type %q", dekInfo.ProtectedData.Type)
	}

	wrapped, err := base64.StdEncoding.DecodeString(dekInfo.ProtectedData.WrappedData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode AWS KMS wrapped data: %w", err)
	}

	client, err := ka.getClient(ctx)
	if err != nil {
		return nil, err
	}

	input := &kms.DecryptInput{
		CiphertextBlob: wrapped,
	}
	if keyID := awsKMSDecryptKeyID(dekInfo.ProtectedData.Metadata); keyID != "" {
		input.KeyId = aws.String(keyID)
	}

	out, err := client.Decrypt(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to unwrap data encryption key with AWS KMS key %s: %w", ka.AwsKMSKeyID, err)
	}
	if len(out.Plaintext) == 0 {
		return nil, errors.New("AWS KMS Decrypt returned empty plaintext")
	}

	return append([]byte(nil), out.Plaintext...), nil
}

func (ka *KeyDataAwsKMS) GetProviderType() ProviderType {
	return ProviderTypeAwsKMS
}

func (ka *KeyDataAwsKMS) getClient(ctx context.Context) (awsKMSClient, error) {
	if ka.clientFactory != nil {
		return ka.clientFactory(ctx)
	}
	return ka.newKMSClient(ctx)
}

func (ka *KeyDataAwsKMS) newKMSClient(ctx context.Context) (*kms.Client, error) {
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

	clientOpts := []func(*kms.Options){}
	if ka.AwsKMSEndpoint != "" {
		clientOpts = append(clientOpts, func(o *kms.Options) {
			o.BaseEndpoint = aws.String(ka.AwsKMSEndpoint)
		})
	}

	return kms.NewFromConfig(cfg, clientOpts...), nil
}

func (ka *KeyDataAwsKMS) providerMetadataFromKeyMetadata(metadata *kmstypes.KeyMetadata) map[string]string {
	result := map[string]string{
		awsKMSMetadataConfiguredKeyID: ka.AwsKMSKeyID,
	}
	if metadata == nil {
		return result
	}
	if metadata.Arn != nil && *metadata.Arn != "" {
		result[awsKMSMetadataKeyARN] = *metadata.Arn
	} else if metadata.KeyId != nil && *metadata.KeyId != "" {
		result[awsKMSMetadataKeyARN] = *metadata.KeyId
	}
	if metadata.CurrentKeyMaterialId != nil && *metadata.CurrentKeyMaterialId != "" {
		result[awsKMSMetadataKeyMaterialID] = *metadata.CurrentKeyMaterialId
	}
	return result
}

func (ka *KeyDataAwsKMS) providerMetadataFromKMSResult(keyID *string, keyMaterialID *string) map[string]string {
	result := map[string]string{
		awsKMSMetadataConfiguredKeyID: ka.AwsKMSKeyID,
	}
	if keyID != nil && *keyID != "" {
		result[awsKMSMetadataKeyARN] = *keyID
	}
	if keyMaterialID != nil && *keyMaterialID != "" {
		result[awsKMSMetadataKeyMaterialID] = *keyMaterialID
	}
	return result
}

func awsKMSProviderVersionFromMetadata(metadata map[string]string) string {
	keyVersionParts := make([]string, 0, 2)
	if keyARN := metadata[awsKMSMetadataKeyARN]; keyARN != "" {
		keyVersionParts = append(keyVersionParts, keyARN)
	}
	if materialID := metadata[awsKMSMetadataKeyMaterialID]; materialID != "" {
		keyVersionParts = append(keyVersionParts, materialID)
	}
	return strings.Join(keyVersionParts, "/")
}

func awsKMSProtectedData(ciphertext []byte, metadata map[string]string) KeyVersionProtectedData {
	return KeyVersionProtectedData{
		Type:        awsKMSProtectedDataType,
		WrappedData: base64.StdEncoding.EncodeToString(ciphertext),
		Metadata:    copyStringMap(metadata),
	}
}

func awsKMSDecryptKeyID(metadata map[string]string) string {
	if keyARN := metadata[awsKMSMetadataKeyARN]; keyARN != "" {
		return keyARN
	}
	return ""
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

var _ KeyDataType = (*KeyDataAwsKMS)(nil)
var _ KeyDataRequiresDataEncryptionKeys = (*KeyDataAwsKMS)(nil)
var _ KeyDataWrapsDataEncryptionKeys = (*KeyDataAwsKMS)(nil)
var _ KeyDataGeneratesDataEncryptionKeys = (*KeyDataAwsKMS)(nil)
