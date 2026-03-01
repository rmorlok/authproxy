package config

import (
	"context"
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

	cache cachedKeyFetcher
}

func (kv *KeyDataVault) HasData(ctx context.Context) bool {
	return kv.VaultAddress != "" && kv.VaultPath != ""
}

func (kv *KeyDataVault) GetData(ctx context.Context) ([]byte, error) {
	kv.cache.fetch = func() ([]byte, error) {
		return kv.fetchFromVault()
	}

	if kv.CacheTTL != "" {
		ttl, err := time.ParseDuration(kv.CacheTTL)
		if err != nil {
			return nil, fmt.Errorf("invalid cache_ttl for vault key data: %w", err)
		}
		kv.cache.ttl = ttl
	}

	return kv.cache.get()
}

func (kv *KeyDataVault) resolveToken() string {
	token := kv.VaultToken
	if token == "" {
		token = os.Getenv("VAULT_TOKEN")
	}
	return token
}

func (kv *KeyDataVault) fetchFromVault() ([]byte, error) {
	config := vault.DefaultConfig()
	config.Address = kv.VaultAddress

	client, err := vault.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create vault client: %w", err)
	}

	token := kv.resolveToken()
	if token != "" {
		client.SetToken(token)
	}

	secret, err := client.Logical().Read(kv.VaultPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read vault path %s: %w", kv.VaultPath, err)
	}

	if secret == nil || secret.Data == nil {
		return nil, fmt.Errorf("no data at vault path %s", kv.VaultPath)
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
		return nil, fmt.Errorf("key %q not found at vault path %s", key, kv.VaultPath)
	}

	strVal, ok := val.(string)
	if !ok {
		return nil, fmt.Errorf("vault key %q at path %s is not a string", key, kv.VaultPath)
	}

	return []byte(strVal), nil
}

var _ KeyDataType = (*KeyDataVault)(nil)
