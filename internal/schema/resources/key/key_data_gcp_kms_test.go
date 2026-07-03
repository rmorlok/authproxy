package key

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"cloud.google.com/go/kms/apiv1/kmspb"
	"github.com/googleapis/gax-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const (
	testGcpKMSKeyName = "projects/test-project/locations/global/keyRings/authproxy/cryptoKeys/dek-wrapper"
	testGcpKMSVersion = testGcpKMSKeyName + "/cryptoKeyVersions/1"
)

type mockGcpKMSClient struct {
	getCryptoKey        func(ctx context.Context, req *kmspb.GetCryptoKeyRequest) (*kmspb.CryptoKey, error)
	generateRandomBytes func(ctx context.Context, req *kmspb.GenerateRandomBytesRequest) (*kmspb.GenerateRandomBytesResponse, error)
	encrypt             func(ctx context.Context, req *kmspb.EncryptRequest) (*kmspb.EncryptResponse, error)
	decrypt             func(ctx context.Context, req *kmspb.DecryptRequest) (*kmspb.DecryptResponse, error)
	close               func() error
}

func (m *mockGcpKMSClient) GetCryptoKey(ctx context.Context, req *kmspb.GetCryptoKeyRequest, _ ...gax.CallOption) (*kmspb.CryptoKey, error) {
	if m.getCryptoKey == nil {
		return nil, errors.New("unexpected GetCryptoKey")
	}
	return m.getCryptoKey(ctx, req)
}

func (m *mockGcpKMSClient) GenerateRandomBytes(ctx context.Context, req *kmspb.GenerateRandomBytesRequest, _ ...gax.CallOption) (*kmspb.GenerateRandomBytesResponse, error) {
	if m.generateRandomBytes == nil {
		return nil, errors.New("unexpected GenerateRandomBytes")
	}
	return m.generateRandomBytes(ctx, req)
}

func (m *mockGcpKMSClient) Encrypt(ctx context.Context, req *kmspb.EncryptRequest, _ ...gax.CallOption) (*kmspb.EncryptResponse, error) {
	if m.encrypt == nil {
		return nil, errors.New("unexpected Encrypt")
	}
	return m.encrypt(ctx, req)
}

func (m *mockGcpKMSClient) Decrypt(ctx context.Context, req *kmspb.DecryptRequest, _ ...gax.CallOption) (*kmspb.DecryptResponse, error) {
	if m.decrypt == nil {
		return nil, errors.New("unexpected Decrypt")
	}
	return m.decrypt(ctx, req)
}

func (m *mockGcpKMSClient) Close() error {
	if m.close != nil {
		return m.close()
	}
	return nil
}

func newTestGcpKMS(keyName string, mock *mockGcpKMSClient) *KeyDataGcpKMS {
	return &KeyDataGcpKMS{
		GcpKMSKeyName: keyName,
		clientFactory: func(context.Context) (gcpKMSClient, error) {
			return mock, nil
		},
	}
}

func TestKeyDataGcpKMS_CurrentWrappingKey(t *testing.T) {
	mock := &mockGcpKMSClient{
		getCryptoKey: func(_ context.Context, req *kmspb.GetCryptoKeyRequest) (*kmspb.CryptoKey, error) {
			require.Equal(t, testGcpKMSKeyName, req.Name)
			return &kmspb.CryptoKey{
				Name: testGcpKMSKeyName,
				Primary: &kmspb.CryptoKeyVersion{
					Name:            testGcpKMSVersion,
					ProtectionLevel: kmspb.ProtectionLevel_HSM,
				},
			}, nil
		},
	}

	kg := newTestGcpKMS(testGcpKMSKeyName, mock)
	current, err := kg.CurrentWrappingKey(context.Background())

	require.NoError(t, err)
	assert.Equal(t, ProviderTypeGcpKMS, current.Provider)
	assert.Equal(t, testGcpKMSKeyName, current.ProviderID)
	assert.Equal(t, testGcpKMSVersion, current.ProviderVersion)
	assert.Equal(t, testGcpKMSKeyName, current.Metadata[gcpKMSMetadataConfiguredKeyName])
	assert.Equal(t, testGcpKMSKeyName, current.Metadata[gcpKMSMetadataCryptoKeyName])
	assert.Equal(t, testGcpKMSVersion, current.Metadata[gcpKMSMetadataCryptoKeyVersion])
	assert.Equal(t, "HSM", current.Metadata[gcpKMSMetadataProtectionLevel])
}

func TestKeyDataGcpKMS_GenerateDataEncryptionKey(t *testing.T) {
	plaintext := []byte("01234567890123456789012345678901")
	ciphertext := []byte("wrapped-by-gcp-kms")
	mock := &mockGcpKMSClient{
		generateRandomBytes: func(_ context.Context, req *kmspb.GenerateRandomBytesRequest) (*kmspb.GenerateRandomBytesResponse, error) {
			require.Equal(t, "projects/test-project/locations/global", req.Location)
			require.Equal(t, int32(DataEncryptionKeySize), req.LengthBytes)
			require.Equal(t, kmspb.ProtectionLevel_HSM, req.ProtectionLevel)
			return &kmspb.GenerateRandomBytesResponse{Data: plaintext}, nil
		},
		getCryptoKey: func(_ context.Context, req *kmspb.GetCryptoKeyRequest) (*kmspb.CryptoKey, error) {
			require.Equal(t, testGcpKMSKeyName, req.Name)
			return &kmspb.CryptoKey{
				Name: testGcpKMSKeyName,
				Primary: &kmspb.CryptoKeyVersion{
					Name:            testGcpKMSVersion,
					ProtectionLevel: kmspb.ProtectionLevel_HSM,
				},
			}, nil
		},
		encrypt: func(_ context.Context, req *kmspb.EncryptRequest) (*kmspb.EncryptResponse, error) {
			require.Equal(t, testGcpKMSKeyName, req.Name)
			require.Equal(t, plaintext, req.Plaintext)
			return &kmspb.EncryptResponse{
				Name:            testGcpKMSVersion,
				Ciphertext:      ciphertext,
				ProtectionLevel: kmspb.ProtectionLevel_HSM,
			}, nil
		},
	}

	kg := &KeyDataGcpKMS{
		GcpProject:   "test-project",
		GcpLocation:  "global",
		GcpKeyRing:   "authproxy",
		GcpCryptoKey: "dek-wrapper",
		clientFactory: func(context.Context) (gcpKMSClient, error) {
			return mock, nil
		},
	}
	generated, err := kg.GenerateDataEncryptionKey(context.Background())

	require.NoError(t, err)
	assert.Equal(t, ProviderTypeGcpKMS, generated.Provider)
	assert.Equal(t, testGcpKMSKeyName, generated.ProviderID)
	assert.Equal(t, testGcpKMSVersion, generated.ProviderVersion)
	assert.Equal(t, plaintext, generated.Data)
	assert.Equal(t, gcpKMSProtectedDataType, generated.ProtectedData.Type)
	assert.Equal(t, testGcpKMSKeyName, generated.ProviderMetadata[gcpKMSMetadataConfiguredKeyName])
	assert.Equal(t, testGcpKMSVersion, generated.ProviderMetadata[gcpKMSMetadataCryptoKeyVersion])

	decoded, err := base64.StdEncoding.DecodeString(generated.ProtectedData.WrappedData)
	require.NoError(t, err)
	assert.Equal(t, ciphertext, decoded)
}

func TestKeyDataGcpKMS_WrapAndUnwrapDataEncryptionKey(t *testing.T) {
	dek := []byte("01234567890123456789012345678901")
	ciphertext := []byte("gcp-kms-ciphertext")
	mock := &mockGcpKMSClient{
		getCryptoKey: func(_ context.Context, _ *kmspb.GetCryptoKeyRequest) (*kmspb.CryptoKey, error) {
			return &kmspb.CryptoKey{
				Name: testGcpKMSKeyName,
				Primary: &kmspb.CryptoKeyVersion{
					Name:            testGcpKMSVersion,
					ProtectionLevel: kmspb.ProtectionLevel_HSM,
				},
			}, nil
		},
		encrypt: func(_ context.Context, req *kmspb.EncryptRequest) (*kmspb.EncryptResponse, error) {
			require.Equal(t, testGcpKMSKeyName, req.Name)
			require.Equal(t, dek, req.Plaintext)
			return &kmspb.EncryptResponse{
				Name:            testGcpKMSVersion,
				Ciphertext:      ciphertext,
				ProtectionLevel: kmspb.ProtectionLevel_HSM,
			}, nil
		},
		decrypt: func(_ context.Context, req *kmspb.DecryptRequest) (*kmspb.DecryptResponse, error) {
			require.Equal(t, testGcpKMSKeyName, req.Name)
			require.Equal(t, ciphertext, req.Ciphertext)
			return &kmspb.DecryptResponse{Plaintext: dek}, nil
		},
	}

	kg := newTestGcpKMS(testGcpKMSKeyName, mock)
	wrapped, err := kg.WrapDataEncryptionKey(context.Background(), dek)
	require.NoError(t, err)
	require.Equal(t, testGcpKMSVersion, wrapped.ProviderVersion)

	// Use a different configured key to prove decrypt follows the persisted DEK metadata.
	unwrapper := newTestGcpKMS("projects/test-project/locations/global/keyRings/authproxy/cryptoKeys/new-wrapper", mock)
	unwrapped, err := unwrapper.UnwrapDataEncryptionKey(context.Background(), DataEncryptionKeyInfo{
		ID:              "dek_test",
		Provider:        ProviderTypeGcpKMS,
		ProviderID:      wrapped.ProviderID,
		ProviderVersion: wrapped.ProviderVersion,
		ProtectedData:   &wrapped.ProtectedData,
	})

	require.NoError(t, err)
	assert.Equal(t, dek, unwrapped)
}

func TestKeyDataGcpKMS_ListVersionsWithDataEncryptionKeys(t *testing.T) {
	plaintext := []byte("01234567890123456789012345678901")
	ciphertext := []byte("gcp-kms-ciphertext")
	protected := gcpKMSProtectedData(ciphertext, map[string]string{
		gcpKMSMetadataCryptoKeyName:    testGcpKMSKeyName,
		gcpKMSMetadataCryptoKeyVersion: testGcpKMSVersion,
	})
	mock := &mockGcpKMSClient{
		decrypt: func(_ context.Context, req *kmspb.DecryptRequest) (*kmspb.DecryptResponse, error) {
			require.Equal(t, testGcpKMSKeyName, req.Name)
			require.Equal(t, ciphertext, req.Ciphertext)
			return &kmspb.DecryptResponse{Plaintext: plaintext}, nil
		},
	}

	kg := newTestGcpKMS(testGcpKMSKeyName, mock)
	versions, err := kg.ListVersionsWithDataEncryptionKeys(context.Background(), []DataEncryptionKeyInfo{
		{
			ID:              "dek_gcp",
			Provider:        ProviderTypeGcpKMS,
			ProviderID:      testGcpKMSKeyName,
			ProviderVersion: testGcpKMSVersion,
			ProtectedData:   &protected,
			IsCurrent:       true,
		},
		{
			ID:       "dek_secret",
			Provider: ProviderTypeGcp,
		},
	})

	require.NoError(t, err)
	require.Len(t, versions, 1)
	assert.Equal(t, "dek_gcp", versions[0].ProviderID)
	assert.Equal(t, plaintext, versions[0].Data)
	assert.True(t, versions[0].IsCurrent)
}

func TestKeyDataGcpKMS_Caching(t *testing.T) {
	callCount := 0
	mock := &mockGcpKMSClient{
		getCryptoKey: func(_ context.Context, _ *kmspb.GetCryptoKeyRequest) (*kmspb.CryptoKey, error) {
			callCount++
			return &kmspb.CryptoKey{
				Name: testGcpKMSKeyName,
				Primary: &kmspb.CryptoKeyVersion{
					Name: testGcpKMSVersion,
				},
			}, nil
		},
	}

	kg := newTestGcpKMS(testGcpKMSKeyName, mock)
	kg.CacheTTL = "1h"

	ctx := context.Background()
	_, err := kg.GetCurrentVersion(ctx)
	require.NoError(t, err)
	_, err = kg.GetCurrentVersion(ctx)
	require.NoError(t, err)

	assert.Equal(t, 1, callCount)
}

func TestKeyDataGcpKMS_Errors(t *testing.T) {
	t.Run("invalid cache ttl", func(t *testing.T) {
		kg := newTestGcpKMS(testGcpKMSKeyName, &mockGcpKMSClient{})
		kg.CacheTTL = "not-a-duration"
		_, err := kg.GetCurrentVersion(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid cache_ttl")
	})

	t.Run("missing key name", func(t *testing.T) {
		kg := &KeyDataGcpKMS{}
		_, err := kg.CurrentWrappingKey(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires either gcp_kms_key_name")
	})

	t.Run("unsupported provider", func(t *testing.T) {
		kg := newTestGcpKMS(testGcpKMSKeyName, &mockGcpKMSClient{})
		_, err := kg.UnwrapDataEncryptionKey(context.Background(), DataEncryptionKeyInfo{
			Provider: ProviderTypeGcp,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported GCP KMS provider")
	})

	t.Run("unsupported protected data type", func(t *testing.T) {
		kg := newTestGcpKMS(testGcpKMSKeyName, &mockGcpKMSClient{})
		_, err := kg.UnwrapDataEncryptionKey(context.Background(), DataEncryptionKeyInfo{
			Provider: ProviderTypeGcpKMS,
			ProtectedData: &KeyVersionProtectedData{
				Type:        "other",
				WrappedData: base64.StdEncoding.EncodeToString([]byte("x")),
			},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported GCP KMS protected data type")
	})

	t.Run("context cancellation", func(t *testing.T) {
		called := false
		kg := &KeyDataGcpKMS{
			GcpKMSKeyName: testGcpKMSKeyName,
			clientFactory: func(context.Context) (gcpKMSClient, error) {
				called = true
				return nil, fmt.Errorf("should not be called")
			},
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := kg.GenerateDataEncryptionKey(ctx)
		require.ErrorIs(t, err, context.Canceled)
		assert.False(t, called)
	})
}

func TestKeyDataGcpKMS_GetProviderType(t *testing.T) {
	kg := &KeyDataGcpKMS{}
	assert.Equal(t, ProviderTypeGcpKMS, kg.GetProviderType())
}

func TestKeyDataGcpKMS_KeyDataSerialization(t *testing.T) {
	t.Run("json", func(t *testing.T) {
		var kd KeyData
		require.NoError(t, json.Unmarshal([]byte(`{
			"gcp_kms_key_name": "projects/test-project/locations/global/keyRings/authproxy/cryptoKeys/dek-wrapper",
			"gcp_kms_endpoint": "localhost:8085",
			"gcp_credentials_json": {"env_var": "GCP_CREDS_JSON"},
			"cache_ttl": "5m"
		}`), &kd))

		gcpKMS, ok := kd.InnerVal.(*KeyDataGcpKMS)
		require.True(t, ok)
		assert.Equal(t, testGcpKMSKeyName, gcpKMS.GcpKMSKeyName)
		assert.Equal(t, "localhost:8085", gcpKMS.GcpKMSEndpoint)
		require.NotNil(t, gcpKMS.GcpCredentialsJSON)
		assert.Equal(t, "5m", gcpKMS.CacheTTL)
	})

	t.Run("yaml", func(t *testing.T) {
		var kd KeyData
		require.NoError(t, yaml.Unmarshal([]byte(`
gcp_project: test-project
gcp_location: global
gcp_key_ring: authproxy
gcp_crypto_key: dek-wrapper
gcp_credentials_file: /tmp/gcp-creds.json
cache_ttl: 5m
`), &kd))

		gcpKMS, ok := kd.InnerVal.(*KeyDataGcpKMS)
		require.True(t, ok)
		assert.Equal(t, "test-project", gcpKMS.GcpProject)
		assert.Equal(t, "global", gcpKMS.GcpLocation)
		assert.Equal(t, "authproxy", gcpKMS.GcpKeyRing)
		assert.Equal(t, "dek-wrapper", gcpKMS.GcpCryptoKey)
		assert.Equal(t, "/tmp/gcp-creds.json", gcpKMS.GcpCredentialsFile)
		assert.Equal(t, "5m", gcpKMS.CacheTTL)
	})
}
