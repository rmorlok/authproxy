package config

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	vault "github.com/hashicorp/vault/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type mockVaultTransitLogical struct {
	read  func(ctx context.Context, path string) (*vault.Secret, error)
	write func(ctx context.Context, path string, data map[string]interface{}) (*vault.Secret, error)
}

func (m *mockVaultTransitLogical) ReadWithContext(ctx context.Context, path string) (*vault.Secret, error) {
	if m.read == nil {
		return nil, errors.New("unexpected ReadWithContext")
	}
	return m.read(ctx, path)
}

func (m *mockVaultTransitLogical) WriteWithContext(ctx context.Context, path string, data map[string]interface{}) (*vault.Secret, error) {
	if m.write == nil {
		return nil, errors.New("unexpected WriteWithContext")
	}
	return m.write(ctx, path, data)
}

func newTestVaultTransit(keyName string, mock *mockVaultTransitLogical) *KeyDataVaultTransit {
	return &KeyDataVaultTransit{
		VaultAddress:        "http://127.0.0.1:8200",
		VaultToken:          "dev-only-token",
		VaultTransitKeyName: keyName,
		clientFactory: func(context.Context) (vaultTransitLogicalClient, error) {
			return mock, nil
		},
	}
}

func TestKeyDataVaultTransit_CurrentWrappingKey(t *testing.T) {
	mock := &mockVaultTransitLogical{
		read: func(_ context.Context, path string) (*vault.Secret, error) {
			require.Equal(t, "transit/keys/authproxy", path)
			return &vault.Secret{Data: map[string]interface{}{
				"latest_version": json.Number("7"),
			}}, nil
		},
	}

	kv := newTestVaultTransit("authproxy", mock)
	current, err := kv.CurrentWrappingKey(context.Background())

	require.NoError(t, err)
	assert.Equal(t, ProviderTypeHashicorpVaultTransit, current.Provider)
	assert.Equal(t, "transit/authproxy", current.ProviderID)
	assert.Equal(t, "7", current.ProviderVersion)
	assert.Equal(t, "transit", current.Metadata[vaultTransitMetadataMountPath])
	assert.Equal(t, "authproxy", current.Metadata[vaultTransitMetadataKeyName])
	assert.Equal(t, "7", current.Metadata[vaultTransitMetadataKeyVersion])
}

func TestKeyDataVaultTransit_GenerateDataEncryptionKey(t *testing.T) {
	plaintext := []byte("01234567890123456789012345678901")
	ciphertext := "vault:v5:wrapped"
	mock := &mockVaultTransitLogical{
		write: func(_ context.Context, path string, data map[string]interface{}) (*vault.Secret, error) {
			require.Equal(t, "transit/datakey/plaintext/authproxy", path)
			require.Equal(t, DataEncryptionKeySize*8, data["bits"])
			return &vault.Secret{Data: map[string]interface{}{
				"plaintext":   base64.StdEncoding.EncodeToString(plaintext),
				"ciphertext":  ciphertext,
				"key_version": json.Number("5"),
			}}, nil
		},
	}

	kv := newTestVaultTransit("authproxy", mock)
	generated, err := kv.GenerateDataEncryptionKey(context.Background())

	require.NoError(t, err)
	assert.Equal(t, ProviderTypeHashicorpVaultTransit, generated.Provider)
	assert.Equal(t, "transit/authproxy", generated.ProviderID)
	assert.Equal(t, "5", generated.ProviderVersion)
	assert.Equal(t, plaintext, generated.Data)
	assert.Equal(t, vaultTransitProtectedDataType, generated.ProtectedData.Type)
	assert.Equal(t, ciphertext, generated.ProtectedData.WrappedData)
	assert.Equal(t, "5", generated.ProviderMetadata[vaultTransitMetadataKeyVersion])
}

func TestKeyDataVaultTransit_WrapAndUnwrapDataEncryptionKey(t *testing.T) {
	dek := []byte("01234567890123456789012345678901")
	ciphertext := "vault:v4:ciphertext"
	mock := &mockVaultTransitLogical{
		read: func(_ context.Context, path string) (*vault.Secret, error) {
			require.Equal(t, "transit/keys/authproxy", path)
			return &vault.Secret{Data: map[string]interface{}{
				"latest_version": json.Number("4"),
			}}, nil
		},
		write: func(_ context.Context, path string, data map[string]interface{}) (*vault.Secret, error) {
			switch path {
			case "transit/encrypt/authproxy":
				require.Equal(t, base64.StdEncoding.EncodeToString(dek), data["plaintext"])
				return &vault.Secret{Data: map[string]interface{}{
					"ciphertext":  ciphertext,
					"key_version": json.Number("4"),
				}}, nil
			case "transit/decrypt/authproxy":
				require.Equal(t, ciphertext, data["ciphertext"])
				return &vault.Secret{Data: map[string]interface{}{
					"plaintext": base64.StdEncoding.EncodeToString(dek),
				}}, nil
			default:
				return nil, fmt.Errorf("unexpected path %s", path)
			}
		},
	}

	kv := newTestVaultTransit("authproxy", mock)
	wrapped, err := kv.WrapDataEncryptionKey(context.Background(), dek)
	require.NoError(t, err)
	require.Equal(t, "4", wrapped.ProviderVersion)

	unwrapper := newTestVaultTransit("new-authproxy", mock)
	unwrapped, err := unwrapper.UnwrapDataEncryptionKey(context.Background(), DataEncryptionKeyInfo{
		ID:              "dek_test",
		Provider:        ProviderTypeHashicorpVaultTransit,
		ProviderID:      wrapped.ProviderID,
		ProviderVersion: wrapped.ProviderVersion,
		ProtectedData:   &wrapped.ProtectedData,
	})

	require.NoError(t, err)
	assert.Equal(t, dek, unwrapped)
}

func TestKeyDataVaultTransit_ListVersionsWithDataEncryptionKeys(t *testing.T) {
	plaintext := []byte("01234567890123456789012345678901")
	protected := vaultTransitProtectedData("vault:v3:ciphertext", map[string]string{
		vaultTransitMetadataMountPath:  "transit",
		vaultTransitMetadataKeyName:    "authproxy",
		vaultTransitMetadataKeyVersion: "3",
	})
	mock := &mockVaultTransitLogical{
		write: func(_ context.Context, path string, data map[string]interface{}) (*vault.Secret, error) {
			require.Equal(t, "transit/decrypt/authproxy", path)
			require.Equal(t, "vault:v3:ciphertext", data["ciphertext"])
			return &vault.Secret{Data: map[string]interface{}{
				"plaintext": base64.StdEncoding.EncodeToString(plaintext),
			}}, nil
		},
	}

	kv := newTestVaultTransit("authproxy", mock)
	versions, err := kv.ListVersionsWithDataEncryptionKeys(context.Background(), []DataEncryptionKeyInfo{
		{
			ID:              "dek_vault_transit",
			Provider:        ProviderTypeHashicorpVaultTransit,
			ProviderID:      "transit/authproxy",
			ProviderVersion: "3",
			ProtectedData:   &protected,
			IsCurrent:       true,
		},
		{
			ID:       "dek_vault_kv",
			Provider: ProviderTypeHashicorpVault,
		},
	})

	require.NoError(t, err)
	require.Len(t, versions, 1)
	assert.Equal(t, "dek_vault_transit", versions[0].ProviderID)
	assert.Equal(t, plaintext, versions[0].Data)
	assert.True(t, versions[0].IsCurrent)
}

func TestKeyDataVaultTransit_Caching(t *testing.T) {
	callCount := 0
	mock := &mockVaultTransitLogical{
		read: func(_ context.Context, _ string) (*vault.Secret, error) {
			callCount++
			return &vault.Secret{Data: map[string]interface{}{
				"latest_version": json.Number("2"),
			}}, nil
		},
	}

	kv := newTestVaultTransit("authproxy", mock)
	kv.CacheTTL = "1h"

	ctx := context.Background()
	_, err := kv.GetCurrentVersion(ctx)
	require.NoError(t, err)
	_, err = kv.GetCurrentVersion(ctx)
	require.NoError(t, err)

	assert.Equal(t, 1, callCount)
}

func TestKeyDataVaultTransit_RetryBehavior(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/transit/keys/authproxy", r.URL.Path)
		callCount++
		if callCount == 1 {
			w.Header().Set("Retry-After", "0.1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"latest_version":9}}`))
	}))
	defer server.Close()

	kv := &KeyDataVaultTransit{
		VaultAddress:        server.URL,
		VaultToken:          "test-token",
		VaultTransitKeyName: "authproxy",
	}
	current, err := kv.CurrentWrappingKey(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "9", current.ProviderVersion)
	assert.Equal(t, 2, callCount)
}

func TestKeyDataVaultTransit_Errors(t *testing.T) {
	t.Run("invalid cache ttl", func(t *testing.T) {
		kv := newTestVaultTransit("authproxy", &mockVaultTransitLogical{})
		kv.CacheTTL = "not-a-duration"
		_, err := kv.GetCurrentVersion(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid cache_ttl")
	})

	t.Run("missing key name", func(t *testing.T) {
		kv := &KeyDataVaultTransit{}
		_, err := kv.CurrentWrappingKey(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "vault_transit_key_name")
	})

	t.Run("unsupported provider", func(t *testing.T) {
		kv := newTestVaultTransit("authproxy", &mockVaultTransitLogical{})
		_, err := kv.UnwrapDataEncryptionKey(context.Background(), DataEncryptionKeyInfo{
			Provider: ProviderTypeHashicorpVault,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported Vault Transit provider")
	})

	t.Run("unsupported protected data type", func(t *testing.T) {
		kv := newTestVaultTransit("authproxy", &mockVaultTransitLogical{})
		_, err := kv.UnwrapDataEncryptionKey(context.Background(), DataEncryptionKeyInfo{
			Provider: ProviderTypeHashicorpVaultTransit,
			ProtectedData: &KeyVersionProtectedData{
				Type:        "other",
				WrappedData: "vault:v1:ciphertext",
			},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported Vault Transit protected data type")
	})

	t.Run("context cancellation", func(t *testing.T) {
		called := false
		kv := &KeyDataVaultTransit{
			VaultAddress:        "http://127.0.0.1:8200",
			VaultTransitKeyName: "authproxy",
			clientFactory: func(context.Context) (vaultTransitLogicalClient, error) {
				called = true
				return nil, fmt.Errorf("should not be called")
			},
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := kv.GenerateDataEncryptionKey(ctx)
		require.ErrorIs(t, err, context.Canceled)
		assert.False(t, called)
	})
}

func TestKeyDataVaultTransit_GetProviderType(t *testing.T) {
	kv := &KeyDataVaultTransit{}
	assert.Equal(t, ProviderTypeHashicorpVaultTransit, kv.GetProviderType())
}

func TestKeyDataVaultTransit_KeyDataSerialization(t *testing.T) {
	t.Run("json", func(t *testing.T) {
		var kd KeyData
		require.NoError(t, json.Unmarshal([]byte(`{
			"vault_address": "http://127.0.0.1:8200",
			"vault_token": "dev-only-token",
			"vault_namespace": "admin",
			"vault_transit_mount_path": "transit",
			"vault_transit_key_name": "authproxy",
			"cache_ttl": "5m"
		}`), &kd))

		transit, ok := kd.InnerVal.(*KeyDataVaultTransit)
		require.True(t, ok)
		assert.Equal(t, "http://127.0.0.1:8200", transit.VaultAddress)
		assert.Equal(t, "dev-only-token", transit.VaultToken)
		assert.Equal(t, "admin", transit.VaultNamespace)
		assert.Equal(t, "transit", transit.VaultTransitMountPath)
		assert.Equal(t, "authproxy", transit.VaultTransitKeyName)
		assert.Equal(t, "5m", transit.CacheTTL)
	})

	t.Run("yaml vault address first", func(t *testing.T) {
		var kd KeyData
		require.NoError(t, yaml.Unmarshal([]byte(`
vault_address: http://127.0.0.1:8200
vault_token: dev-only-token
vault_transit_mount_path: transit
vault_transit_key_name: authproxy
cache_ttl: 5m
`), &kd))

		transit, ok := kd.InnerVal.(*KeyDataVaultTransit)
		require.True(t, ok)
		assert.Equal(t, "http://127.0.0.1:8200", transit.VaultAddress)
		assert.Equal(t, "dev-only-token", transit.VaultToken)
		assert.Equal(t, "transit", transit.VaultTransitMountPath)
		assert.Equal(t, "authproxy", transit.VaultTransitKeyName)
		assert.Equal(t, "5m", transit.CacheTTL)
	})
}
