package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	vault "github.com/hashicorp/vault/api"
)

// KeyDataVault retrieves an AES key from HashiCorp Vault.
type KeyDataVault struct {
	VaultAddress string `json:"vault_address" yaml:"vault_address"`
	VaultToken   string `json:"vault_token" yaml:"vault_token"`
	VaultPath    string `json:"vault_path" yaml:"vault_path"`
	VaultKey     string `json:"vault_key" yaml:"vault_key"`
	CacheTTL     string `json:"cache_ttl,omitempty" yaml:"cache_ttl,omitempty"`

	cache keyDataCache
}

func (kv *KeyDataVault) initCache() error {
	if kv.cache.fetchCurrent != nil {
		return nil
	}

	if kv.CacheTTL != "" {
		ttl, err := time.ParseDuration(kv.CacheTTL)
		if err != nil {
			return fmt.Errorf("invalid cache_ttl for vault key data: %w", err)
		}
		kv.cache.ttl = ttl
	}

	kv.cache.fetchCurrent = kv.fetchCurrentVersion
	kv.cache.fetchVersion = kv.fetchVersionInfo
	kv.cache.fetchList = kv.fetchListVersions
	return nil
}

func (kv *KeyDataVault) vaultProviderID() string {
	key := kv.VaultKey
	if key == "" {
		key = "value"
	}
	return kv.VaultPath + "/" + key
}

func (kv *KeyDataVault) fetchCurrentVersion(ctx context.Context) (KeyVersionInfo, error) {
	data, version, err := kv.fetchFromVault()
	if err != nil {
		return KeyVersionInfo{}, err
	}

	return KeyVersionInfo{
		Provider:        ProviderTypeHashicorpVault,
		ProviderID:      kv.vaultProviderID(),
		ProviderVersion: version,
		Data:            data,
		IsCurrent:       true,
	}, nil
}

func (kv *KeyDataVault) fetchVersionInfo(ctx context.Context, version string) (KeyVersionInfo, error) {
	data, err := kv.fetchVersionFromVault(version)
	if err != nil {
		return KeyVersionInfo{}, err
	}

	return KeyVersionInfo{
		Provider:        ProviderTypeHashicorpVault,
		ProviderID:      kv.vaultProviderID(),
		ProviderVersion: version,
		Data:            data,
		// IsCurrent determined by caller if needed
	}, nil
}

func (kv *KeyDataVault) fetchListVersions(ctx context.Context) ([]KeyVersionInfo, error) {
	v, err := kv.fetchCurrentVersion(ctx)
	if err != nil {
		return nil, err
	}
	return []KeyVersionInfo{v}, nil
}

func (kv *KeyDataVault) GetCurrentVersion(ctx context.Context) (KeyVersionInfo, error) {
	if err := kv.initCache(); err != nil {
		return KeyVersionInfo{}, err
	}
	return kv.cache.GetCurrentVersion(ctx)
}

func (kv *KeyDataVault) GetVersion(ctx context.Context, version string) (KeyVersionInfo, error) {
	if err := kv.initCache(); err != nil {
		return KeyVersionInfo{}, err
	}

	info, err := kv.cache.GetVersion(ctx, version)
	if err != nil {
		return KeyVersionInfo{}, err
	}

	current, _ := kv.cache.GetCurrentVersion(ctx)
	info.IsCurrent = current.ProviderVersion == version
	return info, nil
}

func (kv *KeyDataVault) ListVersions(ctx context.Context) ([]KeyVersionInfo, error) {
	if err := kv.initCache(); err != nil {
		return nil, err
	}
	return kv.cache.ListVersions(ctx)
}

func (kv *KeyDataVault) GetProviderType() ProviderType {
	return ProviderTypeHashicorpVault
}

func (kv *KeyDataVault) resolveToken() string {
	token := kv.VaultToken
	if token == "" {
		token = os.Getenv("VAULT_TOKEN")
	}
	return token
}

func (kv *KeyDataVault) newClient() (*vault.Client, error) {
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

	return client, nil
}

func (kv *KeyDataVault) fetchFromVault() ([]byte, string, error) {
	client, err := kv.newClient()
	if err != nil {
		return nil, "", err
	}

	secret, err := client.Logical().Read(kv.VaultPath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read vault path %s: %w", kv.VaultPath, err)
	}

	if secret == nil || secret.Data == nil {
		return nil, "", fmt.Errorf("no data at vault path %s", kv.VaultPath)
	}

	// KV v2 stores data under a "data" sub-key; extract version from metadata
	var version string
	data := secret.Data
	if d, ok := data["data"].(map[string]interface{}); ok {
		version = extractVaultVersion(data)
		data = d
	}

	key := kv.VaultKey
	if key == "" {
		key = "value"
	}

	val, ok := data[key]
	if !ok {
		return nil, "", fmt.Errorf("key %q not found at vault path %s", key, kv.VaultPath)
	}

	strVal, ok := val.(string)
	if !ok {
		return nil, "", fmt.Errorf("vault key %q at path %s is not a string", key, kv.VaultPath)
	}

	return []byte(strVal), version, nil
}

func (kv *KeyDataVault) fetchVersionFromVault(version string) ([]byte, error) {
	client, err := kv.newClient()
	if err != nil {
		return nil, err
	}

	secret, err := client.Logical().ReadWithData(kv.VaultPath, map[string][]string{
		"version": {version},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to read vault path %s version %s: %w", kv.VaultPath, version, err)
	}

	if secret == nil || secret.Data == nil {
		return nil, fmt.Errorf("no data at vault path %s version %s", kv.VaultPath, version)
	}

	// KV v2 stores data under a "data" sub-key
	data := secret.Data
	if d, ok := data["data"].(map[string]interface{}); ok {
		data = d
	}

	key := kv.VaultKey
	if key == "" {
		key = "value"
	}

	val, ok := data[key]
	if !ok {
		return nil, fmt.Errorf("key %q not found at vault path %s version %s", key, kv.VaultPath, version)
	}

	strVal, ok := val.(string)
	if !ok {
		return nil, fmt.Errorf("vault key %q at path %s version %s is not a string", key, kv.VaultPath, version)
	}

	return []byte(strVal), nil
}

// extractVaultVersion extracts the version number from Vault KV v2 metadata.
func extractVaultVersion(data map[string]interface{}) string {
	metadata, ok := data["metadata"].(map[string]interface{})
	if !ok {
		return ""
	}

	version, ok := metadata["version"]
	if !ok {
		return ""
	}

	// Vault returns version as json.Number or float64 depending on how it's parsed
	switch v := version.(type) {
	case json.Number:
		return v.String()
	case float64:
		return fmt.Sprintf("%d", int64(v))
	case string:
		return v
	}

	return ""
}

var _ KeyDataType = (*KeyDataVault)(nil)
