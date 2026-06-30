package key

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	cloudkms "cloud.google.com/go/kms/apiv1"
	"cloud.google.com/go/kms/apiv1/kmspb"
	"github.com/googleapis/gax-go/v2"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"google.golang.org/api/option"
)

const (
	gcpKMSProtectedDataType = string(ProviderTypeGcpKMS)

	gcpKMSMetadataConfiguredKeyName = "gcp_kms_configured_key_name"
	gcpKMSMetadataCryptoKeyName     = "gcp_kms_crypto_key_name"
	gcpKMSMetadataCryptoKeyVersion  = "gcp_kms_crypto_key_version"
	gcpKMSMetadataProtectionLevel   = "gcp_kms_protection_level"
)

// gcpKMSClient is the subset of the Google Cloud KMS API used by KeyDataGcpKMS.
type gcpKMSClient interface {
	GetCryptoKey(ctx context.Context, req *kmspb.GetCryptoKeyRequest, opts ...gax.CallOption) (*kmspb.CryptoKey, error)
	GenerateRandomBytes(ctx context.Context, req *kmspb.GenerateRandomBytesRequest, opts ...gax.CallOption) (*kmspb.GenerateRandomBytesResponse, error)
	Encrypt(ctx context.Context, req *kmspb.EncryptRequest, opts ...gax.CallOption) (*kmspb.EncryptResponse, error)
	Decrypt(ctx context.Context, req *kmspb.DecryptRequest, opts ...gax.CallOption) (*kmspb.DecryptResponse, error)
	Close() error
}

// KeyDataGcpKMS uses Google Cloud KMS as a wrapping-key provider for data encryption keys.
type KeyDataGcpKMS struct {
	GcpKMSKeyName      string              `json:"gcp_kms_key_name,omitempty" yaml:"gcp_kms_key_name,omitempty"`
	GcpProject         string              `json:"gcp_project,omitempty" yaml:"gcp_project,omitempty"`
	GcpLocation        string              `json:"gcp_location,omitempty" yaml:"gcp_location,omitempty"`
	GcpKeyRing         string              `json:"gcp_key_ring,omitempty" yaml:"gcp_key_ring,omitempty"`
	GcpCryptoKey       string              `json:"gcp_crypto_key,omitempty" yaml:"gcp_crypto_key,omitempty"`
	GcpKMSEndpoint     string              `json:"gcp_kms_endpoint,omitempty" yaml:"gcp_kms_endpoint,omitempty"`
	GcpCredentialsFile string              `json:"gcp_credentials_file,omitempty" yaml:"gcp_credentials_file,omitempty"`
	GcpCredentialsJSON *common.StringValue `json:"gcp_credentials_json,omitempty" yaml:"gcp_credentials_json,omitempty" apiredact:"secret"`
	CacheTTL           string              `json:"cache_ttl,omitempty" yaml:"cache_ttl,omitempty"`

	cache keyDataCache

	// clientFactory overrides the default Google Cloud KMS client creation for testing.
	clientFactory func(ctx context.Context) (gcpKMSClient, error)
}

func (kg *KeyDataGcpKMS) initCache() error {
	if kg.cache.fetchCurrent != nil {
		return nil
	}

	if kg.CacheTTL != "" {
		ttl, err := time.ParseDuration(kg.CacheTTL)
		if err != nil {
			return fmt.Errorf("invalid cache_ttl for gcp kms key data: %w", err)
		}
		kg.cache.ttl = ttl
	}

	kg.cache.fetchCurrent = kg.fetchCurrentVersion
	kg.cache.fetchVersion = kg.fetchVersion
	kg.cache.fetchList = kg.fetchListVersions
	return nil
}

func (kg *KeyDataGcpKMS) providerID() (string, error) {
	return kg.cryptoKeyName()
}

func (kg *KeyDataGcpKMS) fetchCurrentVersion(ctx context.Context) (KeyVersionInfo, error) {
	current, err := kg.CurrentWrappingKey(ctx)
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

func (kg *KeyDataGcpKMS) fetchVersion(ctx context.Context, version string) (KeyVersionInfo, error) {
	current, err := kg.fetchCurrentVersion(ctx)
	if err != nil {
		return KeyVersionInfo{}, err
	}
	if current.ProviderVersion == version {
		return current, nil
	}

	providerID, err := kg.providerID()
	if err != nil {
		return KeyVersionInfo{}, err
	}

	return KeyVersionInfo{
		Provider:        ProviderTypeGcpKMS,
		ProviderID:      providerID,
		ProviderVersion: version,
	}, nil
}

func (kg *KeyDataGcpKMS) fetchListVersions(ctx context.Context) ([]KeyVersionInfo, error) {
	current, err := kg.fetchCurrentVersion(ctx)
	if err != nil {
		return nil, err
	}
	return []KeyVersionInfo{current}, nil
}

func (kg *KeyDataGcpKMS) GetCurrentVersion(ctx context.Context) (KeyVersionInfo, error) {
	if err := kg.initCache(); err != nil {
		return KeyVersionInfo{}, err
	}
	return kg.cache.GetCurrentVersion(ctx)
}

func (kg *KeyDataGcpKMS) GetVersion(ctx context.Context, version string) (KeyVersionInfo, error) {
	if err := kg.initCache(); err != nil {
		return KeyVersionInfo{}, err
	}
	return kg.cache.GetVersion(ctx, version)
}

func (kg *KeyDataGcpKMS) ListVersions(ctx context.Context) ([]KeyVersionInfo, error) {
	if err := kg.initCache(); err != nil {
		return nil, err
	}
	return kg.cache.ListVersions(ctx)
}

func (kg *KeyDataGcpKMS) ListVersionsWithDataEncryptionKeys(ctx context.Context, deks []DataEncryptionKeyInfo) ([]KeyVersionInfo, error) {
	var result []KeyVersionInfo
	for _, dekInfo := range deks {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if dekInfo.Provider != ProviderTypeGcpKMS {
			continue
		}

		dekBytes, err := kg.UnwrapDataEncryptionKey(ctx, dekInfo)
		if err != nil {
			return nil, err
		}

		result = append(result, KeyVersionInfo{
			Provider:        ProviderTypeGcpKMS,
			ProviderID:      dekInfo.ID,
			ProviderVersion: dekInfo.ProviderVersion,
			Data:            dekBytes,
			IsCurrent:       dekInfo.IsCurrent,
		})
	}

	return result, nil
}

func (kg *KeyDataGcpKMS) CurrentWrappingKey(ctx context.Context) (KeyWrappingKeyInfo, error) {
	if err := ctx.Err(); err != nil {
		return KeyWrappingKeyInfo{}, err
	}

	keyName, err := kg.cryptoKeyName()
	if err != nil {
		return KeyWrappingKeyInfo{}, err
	}

	client, err := kg.getClient(ctx)
	if err != nil {
		return KeyWrappingKeyInfo{}, err
	}
	defer client.Close()

	out, err := client.GetCryptoKey(ctx, &kmspb.GetCryptoKeyRequest{
		Name: keyName,
	})
	if err != nil {
		return KeyWrappingKeyInfo{}, fmt.Errorf("failed to get GCP KMS crypto key %s: %w", keyName, err)
	}
	if out == nil {
		return KeyWrappingKeyInfo{}, fmt.Errorf("GCP KMS crypto key %s returned no metadata", keyName)
	}

	metadata := kg.providerMetadataFromCryptoKey(out, keyName)
	version := gcpKMSProviderVersionFromMetadata(metadata)
	if version == "" {
		return KeyWrappingKeyInfo{}, fmt.Errorf("GCP KMS crypto key %s returned no primary version", keyName)
	}

	providerID := gcpKMSCryptoKeyNameFromMetadata(metadata)
	if providerID == "" {
		providerID = keyName
	}

	return KeyWrappingKeyInfo{
		Provider:        ProviderTypeGcpKMS,
		ProviderID:      providerID,
		ProviderVersion: version,
		Metadata:        metadata,
	}, nil
}

func (kg *KeyDataGcpKMS) GenerateDataEncryptionKey(ctx context.Context) (GeneratedDataEncryptionKey, error) {
	if err := ctx.Err(); err != nil {
		return GeneratedDataEncryptionKey{}, err
	}

	location, err := kg.locationName()
	if err != nil {
		return GeneratedDataEncryptionKey{}, err
	}

	client, err := kg.getClient(ctx)
	if err != nil {
		return GeneratedDataEncryptionKey{}, err
	}
	defer client.Close()

	out, err := client.GenerateRandomBytes(ctx, &kmspb.GenerateRandomBytesRequest{
		Location:        location,
		LengthBytes:     DataEncryptionKeySize,
		ProtectionLevel: kmspb.ProtectionLevel_HSM,
	})
	if err != nil {
		return GeneratedDataEncryptionKey{}, fmt.Errorf("failed to generate data encryption key bytes with GCP KMS location %s: %w", location, err)
	}
	if out == nil || len(out.Data) == 0 {
		return GeneratedDataEncryptionKey{}, errors.New("GCP KMS GenerateRandomBytes returned empty data")
	}
	if len(out.Data) != DataEncryptionKeySize {
		return GeneratedDataEncryptionKey{}, fmt.Errorf("GCP KMS GenerateRandomBytes returned %d bytes; expected %d", len(out.Data), DataEncryptionKeySize)
	}

	return kg.WrapDataEncryptionKey(ctx, out.Data)
}

func (kg *KeyDataGcpKMS) WrapDataEncryptionKey(ctx context.Context, dek []byte) (GeneratedDataEncryptionKey, error) {
	if err := ctx.Err(); err != nil {
		return GeneratedDataEncryptionKey{}, err
	}
	if len(dek) == 0 {
		return GeneratedDataEncryptionKey{}, errors.New("data encryption key is empty")
	}

	current, err := kg.CurrentWrappingKey(ctx)
	if err != nil {
		return GeneratedDataEncryptionKey{}, err
	}

	client, err := kg.getClient(ctx)
	if err != nil {
		return GeneratedDataEncryptionKey{}, err
	}
	defer client.Close()

	out, err := client.Encrypt(ctx, &kmspb.EncryptRequest{
		Name:      current.ProviderID,
		Plaintext: dek,
	})
	if err != nil {
		return GeneratedDataEncryptionKey{}, fmt.Errorf("failed to wrap data encryption key with GCP KMS crypto key %s: %w", current.ProviderID, err)
	}
	if out == nil || len(out.Ciphertext) == 0 {
		return GeneratedDataEncryptionKey{}, errors.New("GCP KMS Encrypt returned empty ciphertext")
	}

	metadata := copyStringMap(current.Metadata)
	if out.Name != "" {
		metadata[gcpKMSMetadataCryptoKeyVersion] = out.Name
		if cryptoKeyName := gcpKMSCryptoKeyNameFromVersion(out.Name); cryptoKeyName != "" {
			metadata[gcpKMSMetadataCryptoKeyName] = cryptoKeyName
		}
	}
	if out.ProtectionLevel != kmspb.ProtectionLevel_PROTECTION_LEVEL_UNSPECIFIED {
		metadata[gcpKMSMetadataProtectionLevel] = out.ProtectionLevel.String()
	}

	providerID := gcpKMSCryptoKeyNameFromMetadata(metadata)
	if providerID == "" {
		providerID = current.ProviderID
	}
	version := gcpKMSProviderVersionFromMetadata(metadata)
	if version == "" {
		version = current.ProviderVersion
	}

	return GeneratedDataEncryptionKey{
		Provider:         ProviderTypeGcpKMS,
		ProviderID:       providerID,
		ProviderVersion:  version,
		ProviderMetadata: metadata,
		ProtectedData:    gcpKMSProtectedData(out.Ciphertext, metadata),
		Data:             append([]byte(nil), dek...),
	}, nil
}

func (kg *KeyDataGcpKMS) UnwrapDataEncryptionKey(ctx context.Context, dekInfo DataEncryptionKeyInfo) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if dekInfo.Provider != ProviderTypeGcpKMS {
		return nil, fmt.Errorf("unsupported GCP KMS provider %q", dekInfo.Provider)
	}
	if dekInfo.ProtectedData == nil || dekInfo.ProtectedData.IsZero() {
		return nil, errors.New("data encryption key protected data is empty")
	}
	if dekInfo.ProtectedData.Type != gcpKMSProtectedDataType {
		return nil, fmt.Errorf("unsupported GCP KMS protected data type %q", dekInfo.ProtectedData.Type)
	}

	wrapped, err := base64.StdEncoding.DecodeString(dekInfo.ProtectedData.WrappedData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode GCP KMS wrapped data: %w", err)
	}

	keyName, err := kg.decryptKeyName(dekInfo.ProtectedData.Metadata)
	if err != nil {
		return nil, err
	}

	client, err := kg.getClient(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	out, err := client.Decrypt(ctx, &kmspb.DecryptRequest{
		Name:       keyName,
		Ciphertext: wrapped,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to unwrap data encryption key with GCP KMS crypto key %s: %w", keyName, err)
	}
	if out == nil || len(out.Plaintext) == 0 {
		return nil, errors.New("GCP KMS Decrypt returned empty plaintext")
	}

	return append([]byte(nil), out.Plaintext...), nil
}

func (kg *KeyDataGcpKMS) GetProviderType() ProviderType {
	return ProviderTypeGcpKMS
}

func (kg *KeyDataGcpKMS) getClient(ctx context.Context) (gcpKMSClient, error) {
	if kg.clientFactory != nil {
		return kg.clientFactory(ctx)
	}
	return kg.newKMSClient(ctx)
}

func (kg *KeyDataGcpKMS) newKMSClient(ctx context.Context) (*cloudkms.KeyManagementClient, error) {
	opts := []option.ClientOption{}

	if kg.GcpKMSEndpoint != "" {
		opts = append(opts, option.WithEndpoint(kg.GcpKMSEndpoint))
	}

	if kg.GcpCredentialsFile != "" && kg.GcpCredentialsJSON != nil && kg.GcpCredentialsJSON.HasValue(ctx) {
		return nil, errors.New("only one of gcp_credentials_file or gcp_credentials_json may be configured")
	}
	if kg.GcpCredentialsFile != "" {
		opts = append(opts, option.WithCredentialsFile(kg.GcpCredentialsFile))
	}
	if kg.GcpCredentialsJSON != nil && kg.GcpCredentialsJSON.HasValue(ctx) {
		credsJSON, err := kg.GcpCredentialsJSON.GetValue(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get gcp credentials json: %w", err)
		}
		opts = append(opts, option.WithCredentialsJSON([]byte(credsJSON)))
	}

	client, err := cloudkms.NewKeyManagementClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create gcp kms client: %w", err)
	}
	return client, nil
}

func (kg *KeyDataGcpKMS) cryptoKeyName() (string, error) {
	if kg.GcpKMSKeyName != "" {
		return kg.GcpKMSKeyName, nil
	}
	if kg.GcpProject == "" || kg.GcpLocation == "" || kg.GcpKeyRing == "" || kg.GcpCryptoKey == "" {
		return "", errors.New("gcp kms key data requires either gcp_kms_key_name or gcp_project, gcp_location, gcp_key_ring, and gcp_crypto_key")
	}
	return fmt.Sprintf("projects/%s/locations/%s/keyRings/%s/cryptoKeys/%s", kg.GcpProject, kg.GcpLocation, kg.GcpKeyRing, kg.GcpCryptoKey), nil
}

func (kg *KeyDataGcpKMS) locationName() (string, error) {
	if kg.GcpProject != "" && kg.GcpLocation != "" {
		return fmt.Sprintf("projects/%s/locations/%s", kg.GcpProject, kg.GcpLocation), nil
	}

	keyName, err := kg.cryptoKeyName()
	if err != nil {
		return "", err
	}
	location := gcpKMSLocationNameFromCryptoKey(keyName)
	if location == "" {
		return "", fmt.Errorf("failed to derive gcp kms location from crypto key %s", keyName)
	}
	return location, nil
}

func (kg *KeyDataGcpKMS) decryptKeyName(metadata map[string]string) (string, error) {
	if keyName := gcpKMSCryptoKeyNameFromMetadata(metadata); keyName != "" {
		return keyName, nil
	}
	return kg.cryptoKeyName()
}

func (kg *KeyDataGcpKMS) providerMetadataFromCryptoKey(key *kmspb.CryptoKey, configuredKeyName string) map[string]string {
	result := map[string]string{
		gcpKMSMetadataConfiguredKeyName: configuredKeyName,
	}
	if key == nil {
		return result
	}
	if key.Name != "" {
		result[gcpKMSMetadataCryptoKeyName] = key.Name
	}
	if key.Primary != nil {
		if key.Primary.Name != "" {
			result[gcpKMSMetadataCryptoKeyVersion] = key.Primary.Name
		}
		if key.Primary.ProtectionLevel != kmspb.ProtectionLevel_PROTECTION_LEVEL_UNSPECIFIED {
			result[gcpKMSMetadataProtectionLevel] = key.Primary.ProtectionLevel.String()
		}
	}
	return result
}

func gcpKMSProviderVersionFromMetadata(metadata map[string]string) string {
	return metadata[gcpKMSMetadataCryptoKeyVersion]
}

func gcpKMSCryptoKeyNameFromMetadata(metadata map[string]string) string {
	if keyName := metadata[gcpKMSMetadataCryptoKeyName]; keyName != "" {
		return keyName
	}
	if version := metadata[gcpKMSMetadataCryptoKeyVersion]; version != "" {
		return gcpKMSCryptoKeyNameFromVersion(version)
	}
	return ""
}

func gcpKMSCryptoKeyNameFromVersion(versionName string) string {
	const marker = "/cryptoKeyVersions/"
	idx := strings.LastIndex(versionName, marker)
	if idx < 0 {
		return ""
	}
	return versionName[:idx]
}

func gcpKMSLocationNameFromCryptoKey(keyName string) string {
	const marker = "/keyRings/"
	idx := strings.Index(keyName, marker)
	if idx < 0 {
		return ""
	}
	prefix := keyName[:idx]
	if !strings.Contains(prefix, "/locations/") {
		return ""
	}
	return prefix
}

func gcpKMSProtectedData(ciphertext []byte, metadata map[string]string) KeyVersionProtectedData {
	return KeyVersionProtectedData{
		Type:        gcpKMSProtectedDataType,
		WrappedData: base64.StdEncoding.EncodeToString(ciphertext),
		Metadata:    copyStringMap(metadata),
	}
}

var _ KeyDataType = (*KeyDataGcpKMS)(nil)
var _ KeyDataRequiresDataEncryptionKeys = (*KeyDataGcpKMS)(nil)
var _ KeyDataWrapsDataEncryptionKeys = (*KeyDataGcpKMS)(nil)
var _ KeyDataGeneratesDataEncryptionKeys = (*KeyDataGcpKMS)(nil)
