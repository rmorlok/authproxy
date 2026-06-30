package key

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmstypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type mockKMSClient struct {
	describeKey     func(ctx context.Context, params *kms.DescribeKeyInput) (*kms.DescribeKeyOutput, error)
	generateDataKey func(ctx context.Context, params *kms.GenerateDataKeyInput) (*kms.GenerateDataKeyOutput, error)
	encrypt         func(ctx context.Context, params *kms.EncryptInput) (*kms.EncryptOutput, error)
	decrypt         func(ctx context.Context, params *kms.DecryptInput) (*kms.DecryptOutput, error)
}

func (m *mockKMSClient) DescribeKey(ctx context.Context, params *kms.DescribeKeyInput, _ ...func(*kms.Options)) (*kms.DescribeKeyOutput, error) {
	if m.describeKey == nil {
		return nil, errors.New("unexpected DescribeKey")
	}
	return m.describeKey(ctx, params)
}

func (m *mockKMSClient) GenerateDataKey(ctx context.Context, params *kms.GenerateDataKeyInput, _ ...func(*kms.Options)) (*kms.GenerateDataKeyOutput, error) {
	if m.generateDataKey == nil {
		return nil, errors.New("unexpected GenerateDataKey")
	}
	return m.generateDataKey(ctx, params)
}

func (m *mockKMSClient) Encrypt(ctx context.Context, params *kms.EncryptInput, _ ...func(*kms.Options)) (*kms.EncryptOutput, error) {
	if m.encrypt == nil {
		return nil, errors.New("unexpected Encrypt")
	}
	return m.encrypt(ctx, params)
}

func (m *mockKMSClient) Decrypt(ctx context.Context, params *kms.DecryptInput, _ ...func(*kms.Options)) (*kms.DecryptOutput, error) {
	if m.decrypt == nil {
		return nil, errors.New("unexpected Decrypt")
	}
	return m.decrypt(ctx, params)
}

func newTestAwsKMS(keyID string, mock *mockKMSClient) *KeyDataAwsKMS {
	return &KeyDataAwsKMS{
		AwsKMSKeyID: keyID,
		clientFactory: func(ctx context.Context) (awsKMSClient, error) {
			return mock, nil
		},
	}
}

func TestKeyDataAwsKMS_CurrentWrappingKey(t *testing.T) {
	mock := &mockKMSClient{
		describeKey: func(_ context.Context, params *kms.DescribeKeyInput) (*kms.DescribeKeyOutput, error) {
			require.Equal(t, "alias/authproxy", *params.KeyId)
			return &kms.DescribeKeyOutput{
				KeyMetadata: &kmstypes.KeyMetadata{
					Arn:                  aws.String("arn:aws:kms:us-east-1:123:key/key-1"),
					KeyId:                aws.String("key-1"),
					CurrentKeyMaterialId: aws.String("material-1"),
					KeyState:             kmstypes.KeyStateEnabled,
				},
			}, nil
		},
	}

	ka := newTestAwsKMS("alias/authproxy", mock)
	current, err := ka.CurrentWrappingKey(context.Background())

	require.NoError(t, err)
	assert.Equal(t, ProviderTypeAwsKMS, current.Provider)
	assert.Equal(t, "alias/authproxy", current.ProviderID)
	assert.Equal(t, "arn:aws:kms:us-east-1:123:key/key-1/material-1", current.ProviderVersion)
	assert.Equal(t, "alias/authproxy", current.Metadata[awsKMSMetadataConfiguredKeyID])
	assert.Equal(t, "arn:aws:kms:us-east-1:123:key/key-1", current.Metadata[awsKMSMetadataKeyARN])
	assert.Equal(t, "material-1", current.Metadata[awsKMSMetadataKeyMaterialID])
}

func TestKeyDataAwsKMS_GenerateDataEncryptionKey(t *testing.T) {
	plaintext := []byte("01234567890123456789012345678901")
	ciphertext := []byte("wrapped-by-kms")
	mock := &mockKMSClient{
		generateDataKey: func(_ context.Context, params *kms.GenerateDataKeyInput) (*kms.GenerateDataKeyOutput, error) {
			require.Equal(t, "alias/authproxy", *params.KeyId)
			require.Equal(t, kmstypes.DataKeySpecAes256, params.KeySpec)
			return &kms.GenerateDataKeyOutput{
				KeyId:          aws.String("arn:aws:kms:us-east-1:123:key/key-1"),
				KeyMaterialId:  aws.String("material-1"),
				Plaintext:      plaintext,
				CiphertextBlob: ciphertext,
			}, nil
		},
	}

	ka := newTestAwsKMS("alias/authproxy", mock)
	generated, err := ka.GenerateDataEncryptionKey(context.Background())

	require.NoError(t, err)
	assert.Equal(t, ProviderTypeAwsKMS, generated.Provider)
	assert.Equal(t, "alias/authproxy", generated.ProviderID)
	assert.Equal(t, "arn:aws:kms:us-east-1:123:key/key-1/material-1", generated.ProviderVersion)
	assert.Equal(t, plaintext, generated.Data)
	assert.Equal(t, awsKMSProtectedDataType, generated.ProtectedData.Type)
	assert.Equal(t, "alias/authproxy", generated.ProviderMetadata[awsKMSMetadataConfiguredKeyID])
	assert.Equal(t, "material-1", generated.ProviderMetadata[awsKMSMetadataKeyMaterialID])

	decoded, err := base64.StdEncoding.DecodeString(generated.ProtectedData.WrappedData)
	require.NoError(t, err)
	assert.Equal(t, ciphertext, decoded)
}

func TestKeyDataAwsKMS_WrapAndUnwrapDataEncryptionKey(t *testing.T) {
	dek := []byte("01234567890123456789012345678901")
	ciphertext := []byte("kms-ciphertext")
	mock := &mockKMSClient{
		describeKey: func(_ context.Context, _ *kms.DescribeKeyInput) (*kms.DescribeKeyOutput, error) {
			return &kms.DescribeKeyOutput{
				KeyMetadata: &kmstypes.KeyMetadata{
					Arn:                  aws.String("arn:aws:kms:us-east-1:123:key/key-1"),
					KeyId:                aws.String("key-1"),
					CurrentKeyMaterialId: aws.String("material-1"),
				},
			}, nil
		},
		encrypt: func(_ context.Context, params *kms.EncryptInput) (*kms.EncryptOutput, error) {
			require.Equal(t, "alias/authproxy", *params.KeyId)
			require.Equal(t, dek, params.Plaintext)
			return &kms.EncryptOutput{
				KeyId:          aws.String("arn:aws:kms:us-east-1:123:key/key-1"),
				CiphertextBlob: ciphertext,
			}, nil
		},
		decrypt: func(_ context.Context, params *kms.DecryptInput) (*kms.DecryptOutput, error) {
			require.Equal(t, ciphertext, params.CiphertextBlob)
			require.NotNil(t, params.KeyId)
			require.Equal(t, "arn:aws:kms:us-east-1:123:key/key-1", *params.KeyId)
			return &kms.DecryptOutput{Plaintext: dek}, nil
		},
	}

	ka := newTestAwsKMS("alias/authproxy", mock)
	wrapped, err := ka.WrapDataEncryptionKey(context.Background(), dek)
	require.NoError(t, err)
	require.Equal(t, "arn:aws:kms:us-east-1:123:key/key-1/material-1", wrapped.ProviderVersion)

	unwrapped, err := ka.UnwrapDataEncryptionKey(context.Background(), DataEncryptionKeyInfo{
		ID:              "dek_test",
		Provider:        ProviderTypeAwsKMS,
		ProviderID:      wrapped.ProviderID,
		ProviderVersion: wrapped.ProviderVersion,
		ProtectedData:   &wrapped.ProtectedData,
	})

	require.NoError(t, err)
	assert.Equal(t, dek, unwrapped)
}

func TestKeyDataAwsKMS_ListVersionsWithDataEncryptionKeys(t *testing.T) {
	plaintext := []byte("01234567890123456789012345678901")
	ciphertext := []byte("kms-ciphertext")
	protected := awsKMSProtectedData(ciphertext, map[string]string{
		awsKMSMetadataKeyARN: "arn:aws:kms:us-east-1:123:key/key-1",
	})
	mock := &mockKMSClient{
		decrypt: func(_ context.Context, params *kms.DecryptInput) (*kms.DecryptOutput, error) {
			require.Equal(t, ciphertext, params.CiphertextBlob)
			return &kms.DecryptOutput{Plaintext: plaintext}, nil
		},
	}

	ka := newTestAwsKMS("alias/authproxy", mock)
	versions, err := ka.ListVersionsWithDataEncryptionKeys(context.Background(), []DataEncryptionKeyInfo{
		{
			ID:              "dek_aws",
			Provider:        ProviderTypeAwsKMS,
			ProviderID:      "alias/authproxy",
			ProviderVersion: "v1",
			ProtectedData:   &protected,
			IsCurrent:       true,
		},
		{
			ID:       "dek_secret",
			Provider: ProviderTypeAwsSecretsManager,
		},
	})

	require.NoError(t, err)
	require.Len(t, versions, 1)
	assert.Equal(t, "dek_aws", versions[0].ProviderID)
	assert.Equal(t, plaintext, versions[0].Data)
	assert.True(t, versions[0].IsCurrent)
}

func TestKeyDataAwsKMS_Caching(t *testing.T) {
	callCount := 0
	mock := &mockKMSClient{
		describeKey: func(_ context.Context, _ *kms.DescribeKeyInput) (*kms.DescribeKeyOutput, error) {
			callCount++
			return &kms.DescribeKeyOutput{
				KeyMetadata: &kmstypes.KeyMetadata{
					Arn:                  aws.String("arn:aws:kms:us-east-1:123:key/key-1"),
					KeyId:                aws.String("key-1"),
					CurrentKeyMaterialId: aws.String("material-1"),
				},
			}, nil
		},
	}

	ka := newTestAwsKMS("alias/authproxy", mock)
	ka.CacheTTL = "1h"

	ctx := context.Background()
	_, err := ka.GetCurrentVersion(ctx)
	require.NoError(t, err)
	_, err = ka.GetCurrentVersion(ctx)
	require.NoError(t, err)

	assert.Equal(t, 1, callCount)
}

func TestKeyDataAwsKMS_Errors(t *testing.T) {
	t.Run("invalid cache ttl", func(t *testing.T) {
		ka := newTestAwsKMS("alias/authproxy", &mockKMSClient{})
		ka.CacheTTL = "not-a-duration"
		_, err := ka.GetCurrentVersion(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid cache_ttl")
	})

	t.Run("unsupported provider", func(t *testing.T) {
		ka := newTestAwsKMS("alias/authproxy", &mockKMSClient{})
		_, err := ka.UnwrapDataEncryptionKey(context.Background(), DataEncryptionKeyInfo{
			Provider: ProviderTypeAwsSecretsManager,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported AWS KMS provider")
	})

	t.Run("unsupported protected data type", func(t *testing.T) {
		ka := newTestAwsKMS("alias/authproxy", &mockKMSClient{})
		_, err := ka.UnwrapDataEncryptionKey(context.Background(), DataEncryptionKeyInfo{
			Provider: ProviderTypeAwsKMS,
			ProtectedData: &KeyVersionProtectedData{
				Type:        "other",
				WrappedData: base64.StdEncoding.EncodeToString([]byte("x")),
			},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported AWS KMS protected data type")
	})

	t.Run("context cancellation", func(t *testing.T) {
		called := false
		ka := &KeyDataAwsKMS{
			AwsKMSKeyID: "alias/authproxy",
			clientFactory: func(context.Context) (awsKMSClient, error) {
				called = true
				return nil, fmt.Errorf("should not be called")
			},
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := ka.GenerateDataEncryptionKey(ctx)
		require.ErrorIs(t, err, context.Canceled)
		assert.False(t, called)
	})
}

func TestKeyDataAwsKMS_GetProviderType(t *testing.T) {
	ka := &KeyDataAwsKMS{}
	assert.Equal(t, ProviderTypeAwsKMS, ka.GetProviderType())
}

func TestKeyDataAwsKMS_KeyDataSerialization(t *testing.T) {
	t.Run("json", func(t *testing.T) {
		var kd KeyData
		require.NoError(t, json.Unmarshal([]byte(`{
			"aws_kms_key_id": "alias/authproxy",
			"aws_region": "us-east-1",
			"aws_kms_endpoint": "http://localhost:4566",
			"cache_ttl": "5m"
		}`), &kd))

		awsKMS, ok := kd.InnerVal.(*KeyDataAwsKMS)
		require.True(t, ok)
		assert.Equal(t, "alias/authproxy", awsKMS.AwsKMSKeyID)
		assert.Equal(t, "us-east-1", awsKMS.AwsRegion)
		assert.Equal(t, "http://localhost:4566", awsKMS.AwsKMSEndpoint)
		assert.Equal(t, "5m", awsKMS.CacheTTL)
	})

	t.Run("yaml", func(t *testing.T) {
		var kd KeyData
		require.NoError(t, yaml.Unmarshal([]byte(`
aws_kms_key_id: alias/authproxy
aws_region: us-east-1
aws_kms_endpoint: http://localhost:4566
cache_ttl: 5m
`), &kd))

		awsKMS, ok := kd.InnerVal.(*KeyDataAwsKMS)
		require.True(t, ok)
		assert.Equal(t, "alias/authproxy", awsKMS.AwsKMSKeyID)
		assert.Equal(t, "us-east-1", awsKMS.AwsRegion)
		assert.Equal(t, "http://localhost:4566", awsKMS.AwsKMSEndpoint)
		assert.Equal(t, "5m", awsKMS.CacheTTL)
	})
}
