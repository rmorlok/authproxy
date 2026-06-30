package key

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	vault "github.com/hashicorp/vault/api"
)

const (
	vaultTransitProtectedDataType = string(ProviderTypeHashicorpVaultTransit)

	vaultTransitDefaultMountPath = "transit"

	vaultTransitMetadataMountPath  = "vault_transit_mount_path"
	vaultTransitMetadataKeyName    = "vault_transit_key_name"
	vaultTransitMetadataKeyVersion = "vault_transit_key_version"
	vaultTransitMetadataNamespace  = "vault_namespace"
)

type vaultTransitLogicalClient interface {
	ReadWithContext(ctx context.Context, path string) (*vault.Secret, error)
	WriteWithContext(ctx context.Context, path string, data map[string]interface{}) (*vault.Secret, error)
}

// KeyDataVaultTransit uses HashiCorp Vault Transit as a wrapping-key provider for DEKs.
type KeyDataVaultTransit struct {
	VaultAddress          string `json:"vault_address" yaml:"vault_address"`
	VaultToken            string `json:"vault_token,omitempty" yaml:"vault_token,omitempty" apiredact:"secret"`
	VaultNamespace        string `json:"vault_namespace,omitempty" yaml:"vault_namespace,omitempty"`
	VaultTransitMountPath string `json:"vault_transit_mount_path,omitempty" yaml:"vault_transit_mount_path,omitempty"`
	VaultTransitKeyName   string `json:"vault_transit_key_name" yaml:"vault_transit_key_name"`
	CacheTTL              string `json:"cache_ttl,omitempty" yaml:"cache_ttl,omitempty"`

	cache keyDataCache

	// clientFactory overrides the default Vault client creation for testing.
	clientFactory func(ctx context.Context) (vaultTransitLogicalClient, error)
}

func (kv *KeyDataVaultTransit) initCache() error {
	if kv.cache.fetchCurrent != nil {
		return nil
	}

	if kv.CacheTTL != "" {
		ttl, err := time.ParseDuration(kv.CacheTTL)
		if err != nil {
			return fmt.Errorf("invalid cache_ttl for vault transit key data: %w", err)
		}
		kv.cache.ttl = ttl
	}

	kv.cache.fetchCurrent = kv.fetchCurrentVersion
	kv.cache.fetchVersion = kv.fetchVersion
	kv.cache.fetchList = kv.fetchListVersions
	return nil
}

func (kv *KeyDataVaultTransit) fetchCurrentVersion(ctx context.Context) (KeyVersionInfo, error) {
	current, err := kv.CurrentWrappingKey(ctx)
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

func (kv *KeyDataVaultTransit) fetchVersion(ctx context.Context, version string) (KeyVersionInfo, error) {
	current, err := kv.fetchCurrentVersion(ctx)
	if err != nil {
		return KeyVersionInfo{}, err
	}
	if current.ProviderVersion == version {
		return current, nil
	}

	providerID, err := kv.providerID()
	if err != nil {
		return KeyVersionInfo{}, err
	}

	return KeyVersionInfo{
		Provider:        ProviderTypeHashicorpVaultTransit,
		ProviderID:      providerID,
		ProviderVersion: version,
	}, nil
}

func (kv *KeyDataVaultTransit) fetchListVersions(ctx context.Context) ([]KeyVersionInfo, error) {
	current, err := kv.fetchCurrentVersion(ctx)
	if err != nil {
		return nil, err
	}
	return []KeyVersionInfo{current}, nil
}

func (kv *KeyDataVaultTransit) GetCurrentVersion(ctx context.Context) (KeyVersionInfo, error) {
	if err := kv.initCache(); err != nil {
		return KeyVersionInfo{}, err
	}
	return kv.cache.GetCurrentVersion(ctx)
}

func (kv *KeyDataVaultTransit) GetVersion(ctx context.Context, version string) (KeyVersionInfo, error) {
	if err := kv.initCache(); err != nil {
		return KeyVersionInfo{}, err
	}
	return kv.cache.GetVersion(ctx, version)
}

func (kv *KeyDataVaultTransit) ListVersions(ctx context.Context) ([]KeyVersionInfo, error) {
	if err := kv.initCache(); err != nil {
		return nil, err
	}
	return kv.cache.ListVersions(ctx)
}

func (kv *KeyDataVaultTransit) ListVersionsWithDataEncryptionKeys(ctx context.Context, deks []DataEncryptionKeyInfo) ([]KeyVersionInfo, error) {
	var result []KeyVersionInfo
	for _, dekInfo := range deks {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if dekInfo.Provider != ProviderTypeHashicorpVaultTransit {
			continue
		}

		dekBytes, err := kv.UnwrapDataEncryptionKey(ctx, dekInfo)
		if err != nil {
			return nil, err
		}

		result = append(result, KeyVersionInfo{
			Provider:        ProviderTypeHashicorpVaultTransit,
			ProviderID:      dekInfo.ID,
			ProviderVersion: dekInfo.ProviderVersion,
			Data:            dekBytes,
			IsCurrent:       dekInfo.IsCurrent,
		})
	}

	return result, nil
}

func (kv *KeyDataVaultTransit) CurrentWrappingKey(ctx context.Context) (KeyWrappingKeyInfo, error) {
	if err := ctx.Err(); err != nil {
		return KeyWrappingKeyInfo{}, err
	}

	providerID, err := kv.providerID()
	if err != nil {
		return KeyWrappingKeyInfo{}, err
	}

	client, err := kv.getClient(ctx)
	if err != nil {
		return KeyWrappingKeyInfo{}, err
	}

	secret, err := client.ReadWithContext(ctx, kv.keyInfoPath())
	if err != nil {
		return KeyWrappingKeyInfo{}, fmt.Errorf("failed to read Vault Transit key %s: %w", providerID, err)
	}
	if secret == nil || secret.Data == nil {
		return KeyWrappingKeyInfo{}, fmt.Errorf("no data returned for Vault Transit key %s", providerID)
	}

	version := vaultVersionNumberString(secret.Data["latest_version"])
	if version == "" {
		return KeyWrappingKeyInfo{}, fmt.Errorf("Vault Transit key %s returned no latest_version", providerID)
	}

	metadata := kv.providerMetadata(version)
	return KeyWrappingKeyInfo{
		Provider:        ProviderTypeHashicorpVaultTransit,
		ProviderID:      providerID,
		ProviderVersion: version,
		Metadata:        metadata,
	}, nil
}

func (kv *KeyDataVaultTransit) GenerateDataEncryptionKey(ctx context.Context) (GeneratedDataEncryptionKey, error) {
	if err := ctx.Err(); err != nil {
		return GeneratedDataEncryptionKey{}, err
	}

	providerID, err := kv.providerID()
	if err != nil {
		return GeneratedDataEncryptionKey{}, err
	}

	client, err := kv.getClient(ctx)
	if err != nil {
		return GeneratedDataEncryptionKey{}, err
	}

	secret, err := client.WriteWithContext(ctx, kv.dataKeyPath(), map[string]interface{}{
		"bits": DataEncryptionKeySize * 8,
	})
	if err != nil {
		return GeneratedDataEncryptionKey{}, fmt.Errorf("failed to generate data encryption key with Vault Transit key %s: %w", providerID, err)
	}
	if secret == nil || secret.Data == nil {
		return GeneratedDataEncryptionKey{}, fmt.Errorf("no data returned when generating data encryption key with Vault Transit key %s", providerID)
	}

	plaintextB64, err := vaultTransitString(secret.Data, "plaintext")
	if err != nil {
		return GeneratedDataEncryptionKey{}, err
	}
	dek, err := base64.StdEncoding.DecodeString(plaintextB64)
	if err != nil {
		return GeneratedDataEncryptionKey{}, fmt.Errorf("failed to decode Vault Transit plaintext data key: %w", err)
	}
	if len(dek) != DataEncryptionKeySize {
		return GeneratedDataEncryptionKey{}, fmt.Errorf("Vault Transit returned %d DEK bytes; expected %d", len(dek), DataEncryptionKeySize)
	}

	ciphertext, err := vaultTransitString(secret.Data, "ciphertext")
	if err != nil {
		return GeneratedDataEncryptionKey{}, err
	}

	version := vaultVersionNumberString(secret.Data["key_version"])
	if version == "" {
		current, err := kv.CurrentWrappingKey(ctx)
		if err != nil {
			return GeneratedDataEncryptionKey{}, err
		}
		version = current.ProviderVersion
	}

	metadata := kv.providerMetadata(version)
	return GeneratedDataEncryptionKey{
		Provider:         ProviderTypeHashicorpVaultTransit,
		ProviderID:       providerID,
		ProviderVersion:  version,
		ProviderMetadata: metadata,
		ProtectedData:    vaultTransitProtectedData(ciphertext, metadata),
		Data:             dek,
	}, nil
}

func (kv *KeyDataVaultTransit) WrapDataEncryptionKey(ctx context.Context, dek []byte) (GeneratedDataEncryptionKey, error) {
	if err := ctx.Err(); err != nil {
		return GeneratedDataEncryptionKey{}, err
	}
	if len(dek) == 0 {
		return GeneratedDataEncryptionKey{}, errors.New("data encryption key is empty")
	}

	current, err := kv.CurrentWrappingKey(ctx)
	if err != nil {
		return GeneratedDataEncryptionKey{}, err
	}

	client, err := kv.getClient(ctx)
	if err != nil {
		return GeneratedDataEncryptionKey{}, err
	}

	secret, err := client.WriteWithContext(ctx, kv.encryptPath(), map[string]interface{}{
		"plaintext": base64.StdEncoding.EncodeToString(dek),
	})
	if err != nil {
		return GeneratedDataEncryptionKey{}, fmt.Errorf("failed to wrap data encryption key with Vault Transit key %s: %w", current.ProviderID, err)
	}
	if secret == nil || secret.Data == nil {
		return GeneratedDataEncryptionKey{}, fmt.Errorf("no data returned when wrapping data encryption key with Vault Transit key %s", current.ProviderID)
	}

	ciphertext, err := vaultTransitString(secret.Data, "ciphertext")
	if err != nil {
		return GeneratedDataEncryptionKey{}, err
	}

	version := vaultVersionNumberString(secret.Data["key_version"])
	if version == "" {
		version = current.ProviderVersion
	}
	metadata := kv.providerMetadata(version)

	return GeneratedDataEncryptionKey{
		Provider:         ProviderTypeHashicorpVaultTransit,
		ProviderID:       current.ProviderID,
		ProviderVersion:  version,
		ProviderMetadata: metadata,
		ProtectedData:    vaultTransitProtectedData(ciphertext, metadata),
		Data:             append([]byte(nil), dek...),
	}, nil
}

func (kv *KeyDataVaultTransit) UnwrapDataEncryptionKey(ctx context.Context, dekInfo DataEncryptionKeyInfo) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if dekInfo.Provider != ProviderTypeHashicorpVaultTransit {
		return nil, fmt.Errorf("unsupported Vault Transit provider %q", dekInfo.Provider)
	}
	if dekInfo.ProtectedData == nil || dekInfo.ProtectedData.IsZero() {
		return nil, errors.New("data encryption key protected data is empty")
	}
	if dekInfo.ProtectedData.Type != vaultTransitProtectedDataType {
		return nil, fmt.Errorf("unsupported Vault Transit protected data type %q", dekInfo.ProtectedData.Type)
	}

	decryptPath, err := kv.decryptPathForMetadata(dekInfo.ProtectedData.Metadata)
	if err != nil {
		return nil, err
	}

	client, err := kv.getClient(ctx)
	if err != nil {
		return nil, err
	}

	secret, err := client.WriteWithContext(ctx, decryptPath, map[string]interface{}{
		"ciphertext": dekInfo.ProtectedData.WrappedData,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to unwrap data encryption key with Vault Transit path %s: %w", decryptPath, err)
	}
	if secret == nil || secret.Data == nil {
		return nil, fmt.Errorf("no data returned when unwrapping data encryption key with Vault Transit path %s", decryptPath)
	}

	plaintextB64, err := vaultTransitString(secret.Data, "plaintext")
	if err != nil {
		return nil, err
	}
	dek, err := base64.StdEncoding.DecodeString(plaintextB64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode Vault Transit decrypted data key: %w", err)
	}
	if len(dek) == 0 {
		return nil, errors.New("Vault Transit decrypt returned empty plaintext")
	}

	return dek, nil
}

func (kv *KeyDataVaultTransit) GetProviderType() ProviderType {
	return ProviderTypeHashicorpVaultTransit
}

func (kv *KeyDataVaultTransit) getClient(ctx context.Context) (vaultTransitLogicalClient, error) {
	if kv.clientFactory != nil {
		return kv.clientFactory(ctx)
	}
	return kv.newLogicalClient()
}

func (kv *KeyDataVaultTransit) newLogicalClient() (*vault.Logical, error) {
	config := vault.DefaultConfig()
	config.Address = kv.VaultAddress
	config.HttpClient.Transport = &vaultRetryTransport{base: config.HttpClient.Transport}

	client, err := vault.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create vault client: %w", err)
	}

	token := kv.resolveToken()
	if token != "" {
		client.SetToken(token)
	}
	if kv.VaultNamespace != "" {
		client.SetNamespace(kv.VaultNamespace)
	}

	return client.Logical(), nil
}

func (kv *KeyDataVaultTransit) resolveToken() string {
	token := kv.VaultToken
	if token == "" {
		token = os.Getenv("VAULT_TOKEN")
	}
	return token
}

func (kv *KeyDataVaultTransit) providerID() (string, error) {
	mount := kv.mountPath()
	keyName := kv.keyName()
	if keyName == "" {
		return "", errors.New("vault transit key data requires vault_transit_key_name")
	}
	return mount + "/" + keyName, nil
}

func (kv *KeyDataVaultTransit) mountPath() string {
	mount := strings.Trim(kv.VaultTransitMountPath, "/")
	if mount == "" {
		return vaultTransitDefaultMountPath
	}
	return mount
}

func (kv *KeyDataVaultTransit) keyName() string {
	return strings.Trim(kv.VaultTransitKeyName, "/")
}

func (kv *KeyDataVaultTransit) keyInfoPath() string {
	return kv.mountPath() + "/keys/" + kv.keyName()
}

func (kv *KeyDataVaultTransit) dataKeyPath() string {
	return kv.mountPath() + "/datakey/plaintext/" + kv.keyName()
}

func (kv *KeyDataVaultTransit) encryptPath() string {
	return kv.mountPath() + "/encrypt/" + kv.keyName()
}

func (kv *KeyDataVaultTransit) decryptPath() string {
	return kv.mountPath() + "/decrypt/" + kv.keyName()
}

func (kv *KeyDataVaultTransit) decryptPathForMetadata(metadata map[string]string) (string, error) {
	mount := metadata[vaultTransitMetadataMountPath]
	keyName := metadata[vaultTransitMetadataKeyName]
	if mount == "" || keyName == "" {
		return kv.decryptPath(), nil
	}
	return strings.Trim(mount, "/") + "/decrypt/" + strings.Trim(keyName, "/"), nil
}

func (kv *KeyDataVaultTransit) providerMetadata(version string) map[string]string {
	metadata := map[string]string{
		vaultTransitMetadataMountPath:  kv.mountPath(),
		vaultTransitMetadataKeyName:    kv.keyName(),
		vaultTransitMetadataKeyVersion: version,
	}
	if kv.VaultNamespace != "" {
		metadata[vaultTransitMetadataNamespace] = kv.VaultNamespace
	}
	return metadata
}

func vaultTransitString(data map[string]interface{}, key string) (string, error) {
	val, ok := data[key]
	if !ok {
		return "", fmt.Errorf("Vault Transit response missing %q", key)
	}
	str, ok := val.(string)
	if !ok || str == "" {
		return "", fmt.Errorf("Vault Transit response %q is not a non-empty string", key)
	}
	return str, nil
}

func vaultTransitProtectedData(ciphertext string, metadata map[string]string) KeyVersionProtectedData {
	return KeyVersionProtectedData{
		Type:        vaultTransitProtectedDataType,
		WrappedData: ciphertext,
		Metadata:    copyStringMap(metadata),
	}
}

var _ KeyDataType = (*KeyDataVaultTransit)(nil)
var _ KeyDataRequiresDataEncryptionKeys = (*KeyDataVaultTransit)(nil)
var _ KeyDataWrapsDataEncryptionKeys = (*KeyDataVaultTransit)(nil)
var _ KeyDataGeneratesDataEncryptionKeys = (*KeyDataVaultTransit)(nil)
