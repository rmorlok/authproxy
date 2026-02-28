package config

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	smtypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSMClient implements awsSecretsManagerClient for testing.
type mockSMClient struct {
	getSecretValue      func(ctx context.Context, params *secretsmanager.GetSecretValueInput) (*secretsmanager.GetSecretValueOutput, error)
	listSecretVersionIds func(ctx context.Context, params *secretsmanager.ListSecretVersionIdsInput) (*secretsmanager.ListSecretVersionIdsOutput, error)
}

func (m *mockSMClient) GetSecretValue(ctx context.Context, params *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	return m.getSecretValue(ctx, params)
}

func (m *mockSMClient) ListSecretVersionIds(ctx context.Context, params *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
	return m.listSecretVersionIds(ctx, params)
}

func newTestAwsSecret(secretID string, mock *mockSMClient) *KeyDataAwsSecret {
	return &KeyDataAwsSecret{
		AwsSecretID: secretID,
		clientFactory: func(ctx context.Context) (awsSecretsManagerClient, error) {
			return mock, nil
		},
	}
}

func TestKeyDataAwsSecret_GetCurrentVersion(t *testing.T) {
	t.Run("returns secret string with version", func(t *testing.T) {
		mock := &mockSMClient{
			getSecretValue: func(_ context.Context, params *secretsmanager.GetSecretValueInput) (*secretsmanager.GetSecretValueOutput, error) {
				assert.Equal(t, "my-secret", *params.SecretId)
				assert.Nil(t, params.VersionId)
				return &secretsmanager.GetSecretValueOutput{
					SecretString: aws.String("my-secret-value"),
					VersionId:    aws.String("v1"),
				}, nil
			},
		}

		ka := newTestAwsSecret("my-secret", mock)
		info, err := ka.GetCurrentVersion(context.Background())

		require.NoError(t, err)
		assert.Equal(t, []byte("my-secret-value"), info.Data)
		assert.Equal(t, "v1", info.ProviderVersion)
		assert.Equal(t, ProviderTypeAws, info.Provider)
		assert.Equal(t, "my-secret", info.ProviderID)
		assert.True(t, info.IsCurrent)
	})

	t.Run("returns binary secret", func(t *testing.T) {
		binaryData := []byte{0x01, 0x02, 0x03, 0x04}
		mock := &mockSMClient{
			getSecretValue: func(_ context.Context, _ *secretsmanager.GetSecretValueInput) (*secretsmanager.GetSecretValueOutput, error) {
				return &secretsmanager.GetSecretValueOutput{
					SecretBinary: binaryData,
					VersionId:    aws.String("v1"),
				}, nil
			},
		}

		ka := newTestAwsSecret("my-secret", mock)
		info, err := ka.GetCurrentVersion(context.Background())

		require.NoError(t, err)
		assert.Equal(t, binaryData, info.Data)
	})

	t.Run("prefers binary over string", func(t *testing.T) {
		binaryData := []byte{0xDE, 0xAD}
		mock := &mockSMClient{
			getSecretValue: func(_ context.Context, _ *secretsmanager.GetSecretValueInput) (*secretsmanager.GetSecretValueOutput, error) {
				return &secretsmanager.GetSecretValueOutput{
					SecretString: aws.String("string-value"),
					SecretBinary: binaryData,
					VersionId:    aws.String("v1"),
				}, nil
			},
		}

		ka := newTestAwsSecret("my-secret", mock)
		info, err := ka.GetCurrentVersion(context.Background())

		require.NoError(t, err)
		assert.Equal(t, binaryData, info.Data)
	})

	t.Run("error when no string or binary value", func(t *testing.T) {
		mock := &mockSMClient{
			getSecretValue: func(_ context.Context, _ *secretsmanager.GetSecretValueInput) (*secretsmanager.GetSecretValueOutput, error) {
				return &secretsmanager.GetSecretValueOutput{
					VersionId: aws.String("v1"),
				}, nil
			},
		}

		ka := newTestAwsSecret("my-secret", mock)
		_, err := ka.GetCurrentVersion(context.Background())

		require.Error(t, err)
		assert.Contains(t, err.Error(), "has no string or binary value")
	})

	t.Run("propagates GetSecretValue error", func(t *testing.T) {
		mock := &mockSMClient{
			getSecretValue: func(_ context.Context, _ *secretsmanager.GetSecretValueInput) (*secretsmanager.GetSecretValueOutput, error) {
				return nil, fmt.Errorf("access denied")
			},
		}

		ka := newTestAwsSecret("my-secret", mock)
		_, err := ka.GetCurrentVersion(context.Background())

		require.Error(t, err)
		assert.Contains(t, err.Error(), "access denied")
	})

	t.Run("handles nil VersionId in response", func(t *testing.T) {
		mock := &mockSMClient{
			getSecretValue: func(_ context.Context, _ *secretsmanager.GetSecretValueInput) (*secretsmanager.GetSecretValueOutput, error) {
				return &secretsmanager.GetSecretValueOutput{
					SecretString: aws.String("value"),
				}, nil
			},
		}

		ka := newTestAwsSecret("my-secret", mock)
		info, err := ka.GetCurrentVersion(context.Background())

		require.NoError(t, err)
		assert.Equal(t, "", info.ProviderVersion)
	})

	t.Run("invalid cache_ttl returns error", func(t *testing.T) {
		mock := &mockSMClient{}
		ka := newTestAwsSecret("my-secret", mock)
		ka.CacheTTL = "not-a-duration"

		_, err := ka.GetCurrentVersion(context.Background())

		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid cache_ttl")
	})
}

func TestKeyDataAwsSecret_GetCurrentVersion_WithSecretKey(t *testing.T) {
	t.Run("extracts JSON key from secret string", func(t *testing.T) {
		mock := &mockSMClient{
			getSecretValue: func(_ context.Context, _ *secretsmanager.GetSecretValueInput) (*secretsmanager.GetSecretValueOutput, error) {
				return &secretsmanager.GetSecretValueOutput{
					SecretString: aws.String(`{"db_password":"s3cret","api_key":"abc123"}`),
					VersionId:    aws.String("v2"),
				}, nil
			},
		}

		ka := newTestAwsSecret("my-secret", mock)
		ka.AwsSecretKey = "db_password"

		info, err := ka.GetCurrentVersion(context.Background())

		require.NoError(t, err)
		assert.Equal(t, []byte("s3cret"), info.Data)
		assert.Equal(t, "my-secret/db_password", info.ProviderID)
	})

	t.Run("error when JSON key not found", func(t *testing.T) {
		mock := &mockSMClient{
			getSecretValue: func(_ context.Context, _ *secretsmanager.GetSecretValueInput) (*secretsmanager.GetSecretValueOutput, error) {
				return &secretsmanager.GetSecretValueOutput{
					SecretString: aws.String(`{"other_key":"value"}`),
					VersionId:    aws.String("v1"),
				}, nil
			},
		}

		ka := newTestAwsSecret("my-secret", mock)
		ka.AwsSecretKey = "missing_key"

		_, err := ka.GetCurrentVersion(context.Background())

		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("error when secret string is nil with secret key", func(t *testing.T) {
		mock := &mockSMClient{
			getSecretValue: func(_ context.Context, _ *secretsmanager.GetSecretValueInput) (*secretsmanager.GetSecretValueOutput, error) {
				return &secretsmanager.GetSecretValueOutput{
					SecretBinary: []byte{0x01},
					VersionId:    aws.String("v1"),
				}, nil
			},
		}

		ka := newTestAwsSecret("my-secret", mock)
		ka.AwsSecretKey = "my_key"

		_, err := ka.GetCurrentVersion(context.Background())

		require.Error(t, err)
		assert.Contains(t, err.Error(), "has no string value")
	})

	t.Run("error when secret string is not valid JSON", func(t *testing.T) {
		mock := &mockSMClient{
			getSecretValue: func(_ context.Context, _ *secretsmanager.GetSecretValueInput) (*secretsmanager.GetSecretValueOutput, error) {
				return &secretsmanager.GetSecretValueOutput{
					SecretString: aws.String("not json"),
					VersionId:    aws.String("v1"),
				}, nil
			},
		}

		ka := newTestAwsSecret("my-secret", mock)
		ka.AwsSecretKey = "my_key"

		_, err := ka.GetCurrentVersion(context.Background())

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse")
	})
}

func TestKeyDataAwsSecret_GetVersion(t *testing.T) {
	t.Run("fetches specific version", func(t *testing.T) {
		mock := &mockSMClient{
			getSecretValue: func(_ context.Context, params *secretsmanager.GetSecretValueInput) (*secretsmanager.GetSecretValueOutput, error) {
				if params.VersionId != nil && *params.VersionId == "v2" {
					return &secretsmanager.GetSecretValueOutput{
						SecretString: aws.String("old-value"),
						VersionId:    aws.String("v2"),
					}, nil
				}
				// Current version
				return &secretsmanager.GetSecretValueOutput{
					SecretString: aws.String("current-value"),
					VersionId:    aws.String("v3"),
				}, nil
			},
		}

		ka := newTestAwsSecret("my-secret", mock)

		info, err := ka.GetVersion(context.Background(), "v2")

		require.NoError(t, err)
		assert.Equal(t, []byte("old-value"), info.Data)
		assert.Equal(t, "v2", info.ProviderVersion)
		assert.False(t, info.IsCurrent)
	})

	t.Run("marks as current when version matches current", func(t *testing.T) {
		mock := &mockSMClient{
			getSecretValue: func(_ context.Context, _ *secretsmanager.GetSecretValueInput) (*secretsmanager.GetSecretValueOutput, error) {
				return &secretsmanager.GetSecretValueOutput{
					SecretString: aws.String("current-value"),
					VersionId:    aws.String("v3"),
				}, nil
			},
		}

		ka := newTestAwsSecret("my-secret", mock)

		info, err := ka.GetVersion(context.Background(), "v3")

		require.NoError(t, err)
		assert.True(t, info.IsCurrent)
	})

	t.Run("propagates error", func(t *testing.T) {
		mock := &mockSMClient{
			getSecretValue: func(_ context.Context, params *secretsmanager.GetSecretValueInput) (*secretsmanager.GetSecretValueOutput, error) {
				if params.VersionId != nil {
					return nil, fmt.Errorf("version not found")
				}
				return &secretsmanager.GetSecretValueOutput{
					SecretString: aws.String("value"),
					VersionId:    aws.String("v1"),
				}, nil
			},
		}

		ka := newTestAwsSecret("my-secret", mock)

		_, err := ka.GetVersion(context.Background(), "v-nonexistent")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "version not found")
	})
}

func TestKeyDataAwsSecret_ListVersions(t *testing.T) {
	t.Run("lists all versions with correct IsCurrent", func(t *testing.T) {
		mock := &mockSMClient{
			listSecretVersionIds: func(_ context.Context, _ *secretsmanager.ListSecretVersionIdsInput) (*secretsmanager.ListSecretVersionIdsOutput, error) {
				return &secretsmanager.ListSecretVersionIdsOutput{
					Versions: []smtypes.SecretVersionsListEntry{
						{VersionId: aws.String("v1"), VersionStages: []string{"AWSPREVIOUS"}},
						{VersionId: aws.String("v2"), VersionStages: []string{"AWSCURRENT"}},
					},
				}, nil
			},
			getSecretValue: func(_ context.Context, params *secretsmanager.GetSecretValueInput) (*secretsmanager.GetSecretValueOutput, error) {
				vid := "v1"
				if params.VersionId != nil {
					vid = *params.VersionId
				}
				return &secretsmanager.GetSecretValueOutput{
					SecretString: aws.String("value-" + vid),
					VersionId:    aws.String(vid),
				}, nil
			},
		}

		ka := newTestAwsSecret("my-secret", mock)

		infos, err := ka.ListVersions(context.Background())

		require.NoError(t, err)
		require.Len(t, infos, 2)

		assert.Equal(t, "v1", infos[0].ProviderVersion)
		assert.Equal(t, []byte("value-v1"), infos[0].Data)
		assert.False(t, infos[0].IsCurrent)

		assert.Equal(t, "v2", infos[1].ProviderVersion)
		assert.Equal(t, []byte("value-v2"), infos[1].Data)
		assert.True(t, infos[1].IsCurrent)
	})

	t.Run("paginates through all versions", func(t *testing.T) {
		page2Token := "page2"
		mock := &mockSMClient{
			listSecretVersionIds: func(_ context.Context, params *secretsmanager.ListSecretVersionIdsInput) (*secretsmanager.ListSecretVersionIdsOutput, error) {
				if params.NextToken == nil {
					return &secretsmanager.ListSecretVersionIdsOutput{
						Versions:  []smtypes.SecretVersionsListEntry{{VersionId: aws.String("v1"), VersionStages: []string{"AWSPREVIOUS"}}},
						NextToken: &page2Token,
					}, nil
				}
				return &secretsmanager.ListSecretVersionIdsOutput{
					Versions: []smtypes.SecretVersionsListEntry{{VersionId: aws.String("v2"), VersionStages: []string{"AWSCURRENT"}}},
				}, nil
			},
			getSecretValue: func(_ context.Context, params *secretsmanager.GetSecretValueInput) (*secretsmanager.GetSecretValueOutput, error) {
				vid := *params.VersionId
				return &secretsmanager.GetSecretValueOutput{
					SecretString: aws.String("val-" + vid),
					VersionId:    aws.String(vid),
				}, nil
			},
		}

		ka := newTestAwsSecret("my-secret", mock)

		infos, err := ka.ListVersions(context.Background())

		require.NoError(t, err)
		require.Len(t, infos, 2)
		assert.Equal(t, "v1", infos[0].ProviderVersion)
		assert.Equal(t, "v2", infos[1].ProviderVersion)
	})

	t.Run("skips versions with nil VersionId", func(t *testing.T) {
		mock := &mockSMClient{
			listSecretVersionIds: func(_ context.Context, _ *secretsmanager.ListSecretVersionIdsInput) (*secretsmanager.ListSecretVersionIdsOutput, error) {
				return &secretsmanager.ListSecretVersionIdsOutput{
					Versions: []smtypes.SecretVersionsListEntry{
						{VersionId: nil, VersionStages: []string{"AWSCURRENT"}},
						{VersionId: aws.String("v1"), VersionStages: []string{"AWSPREVIOUS"}},
					},
				}, nil
			},
			getSecretValue: func(_ context.Context, params *secretsmanager.GetSecretValueInput) (*secretsmanager.GetSecretValueOutput, error) {
				return &secretsmanager.GetSecretValueOutput{
					SecretString: aws.String("data"),
					VersionId:    params.VersionId,
				}, nil
			},
		}

		ka := newTestAwsSecret("my-secret", mock)

		infos, err := ka.ListVersions(context.Background())

		require.NoError(t, err)
		require.Len(t, infos, 1)
		assert.Equal(t, "v1", infos[0].ProviderVersion)
	})

	t.Run("propagates ListSecretVersionIds error", func(t *testing.T) {
		mock := &mockSMClient{
			listSecretVersionIds: func(_ context.Context, _ *secretsmanager.ListSecretVersionIdsInput) (*secretsmanager.ListSecretVersionIdsOutput, error) {
				return nil, fmt.Errorf("throttled")
			},
		}

		ka := newTestAwsSecret("my-secret", mock)

		_, err := ka.ListVersions(context.Background())

		require.Error(t, err)
		assert.Contains(t, err.Error(), "throttled")
	})

	t.Run("propagates GetSecretValue error during list", func(t *testing.T) {
		mock := &mockSMClient{
			listSecretVersionIds: func(_ context.Context, _ *secretsmanager.ListSecretVersionIdsInput) (*secretsmanager.ListSecretVersionIdsOutput, error) {
				return &secretsmanager.ListSecretVersionIdsOutput{
					Versions: []smtypes.SecretVersionsListEntry{
						{VersionId: aws.String("v1"), VersionStages: []string{"AWSCURRENT"}},
					},
				}, nil
			},
			getSecretValue: func(_ context.Context, _ *secretsmanager.GetSecretValueInput) (*secretsmanager.GetSecretValueOutput, error) {
				return nil, fmt.Errorf("secret deleted")
			},
		}

		ka := newTestAwsSecret("my-secret", mock)

		_, err := ka.ListVersions(context.Background())

		require.Error(t, err)
		assert.Contains(t, err.Error(), "secret deleted")
	})

	t.Run("empty version list", func(t *testing.T) {
		mock := &mockSMClient{
			listSecretVersionIds: func(_ context.Context, _ *secretsmanager.ListSecretVersionIdsInput) (*secretsmanager.ListSecretVersionIdsOutput, error) {
				return &secretsmanager.ListSecretVersionIdsOutput{
					Versions: []smtypes.SecretVersionsListEntry{},
				}, nil
			},
		}

		ka := newTestAwsSecret("my-secret", mock)

		infos, err := ka.ListVersions(context.Background())

		require.NoError(t, err)
		assert.Empty(t, infos)
	})
}

func TestKeyDataAwsSecret_Caching(t *testing.T) {
	t.Run("caches GetCurrentVersion result", func(t *testing.T) {
		callCount := 0
		mock := &mockSMClient{
			getSecretValue: func(_ context.Context, _ *secretsmanager.GetSecretValueInput) (*secretsmanager.GetSecretValueOutput, error) {
				callCount++
				return &secretsmanager.GetSecretValueOutput{
					SecretString: aws.String("value"),
					VersionId:    aws.String("v1"),
				}, nil
			},
		}

		ka := newTestAwsSecret("my-secret", mock)
		ka.CacheTTL = "1h"

		ctx := context.Background()
		_, err := ka.GetCurrentVersion(ctx)
		require.NoError(t, err)

		_, err = ka.GetCurrentVersion(ctx)
		require.NoError(t, err)

		assert.Equal(t, 1, callCount, "should only call AWS once due to caching")
	})

	t.Run("caches GetVersion result", func(t *testing.T) {
		callCount := 0
		mock := &mockSMClient{
			getSecretValue: func(_ context.Context, params *secretsmanager.GetSecretValueInput) (*secretsmanager.GetSecretValueOutput, error) {
				callCount++
				vid := "v1"
				if params.VersionId != nil {
					vid = *params.VersionId
				}
				return &secretsmanager.GetSecretValueOutput{
					SecretString: aws.String("value"),
					VersionId:    aws.String(vid),
				}, nil
			},
		}

		ka := newTestAwsSecret("my-secret", mock)
		ka.CacheTTL = "1h"

		ctx := context.Background()
		// First call fetches v2 + current (for IsCurrent check) = 2 calls
		_, err := ka.GetVersion(ctx, "v2")
		require.NoError(t, err)

		// Second call should be fully cached = still 2 calls
		_, err = ka.GetVersion(ctx, "v2")
		require.NoError(t, err)

		assert.Equal(t, 2, callCount, "should cache version results")
	})
}

func TestKeyDataAwsSecret_GetProviderType(t *testing.T) {
	ka := &KeyDataAwsSecret{}
	assert.Equal(t, ProviderTypeAws, ka.GetProviderType())
}

func TestKeyDataAwsSecret_ClientFactoryError(t *testing.T) {
	ka := &KeyDataAwsSecret{
		AwsSecretID: "my-secret",
		clientFactory: func(_ context.Context) (awsSecretsManagerClient, error) {
			return nil, fmt.Errorf("client creation failed")
		},
	}

	ctx := context.Background()

	t.Run("GetCurrentVersion", func(t *testing.T) {
		_, err := ka.GetCurrentVersion(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "client creation failed")
	})

	t.Run("ListVersions", func(t *testing.T) {
		// Reset cache so fetchList is triggered
		ka.cache = keyDataCache{}
		_, err := ka.ListVersions(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "client creation failed")
	})
}
