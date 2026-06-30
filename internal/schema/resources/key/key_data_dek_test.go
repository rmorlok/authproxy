package key

import (
	"context"
	"testing"

	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
)

func TestKeyDataAuthProxyGeneratedDataEncryptionKey(t *testing.T) {
	ctx := context.Background()

	ResetKeyDataMockRegistry()
	t.Cleanup(ResetKeyDataMockRegistry)

	kd := NewKeyDataMock("authproxy-generated-dek")
	wrappingKey := util.MustGenerateSecureRandomKey(32)
	KeyDataMockAddVersion("authproxy-generated-dek", "mock-key", "v1", wrappingKey)

	current, err := kd.CurrentWrappingKey(ctx)
	require.NoError(t, err)
	require.Equal(t, ProviderTypeMock, current.Provider)
	require.Equal(t, "mock-key", current.ProviderID)
	require.Equal(t, "v1", current.ProviderVersion)

	generated, err := kd.GenerateDataEncryptionKey(ctx)
	require.NoError(t, err)
	require.Equal(t, ProviderTypeMock, generated.Provider)
	require.Equal(t, "mock-key", generated.ProviderID)
	require.Equal(t, "v1", generated.ProviderVersion)
	require.Len(t, generated.Data, DataEncryptionKeySize)
	require.Equal(t, KeyVersionProtectedDataTypeAuthProxyAESGCM, generated.ProtectedData.Type)
	require.Equal(t, string(ProviderTypeMock), generated.ProtectedData.Metadata[protectedDataMetadataWrappingProvider])
	require.Equal(t, "mock-key", generated.ProtectedData.Metadata[protectedDataMetadataWrappingProviderID])
	require.Equal(t, "v1", generated.ProtectedData.Metadata[protectedDataMetadataWrappingProviderVersion])
	require.NotEmpty(t, generated.ProtectedData.WrappedData)

	unwrapped, err := kd.UnwrapDataEncryptionKey(ctx, DataEncryptionKeyInfo{
		ID:              "dek_generated",
		EncryptionKeyID: "key_generated",
		Provider:        generated.Provider,
		ProviderID:      generated.ProviderID,
		ProviderVersion: generated.ProviderVersion,
		ProtectedData:   &generated.ProtectedData,
	})
	require.NoError(t, err)
	require.Equal(t, generated.Data, unwrapped)
}

func TestKeyDataWrapsExplicitDataEncryptionKey(t *testing.T) {
	ctx := context.Background()

	ResetKeyDataMockRegistry()
	t.Cleanup(ResetKeyDataMockRegistry)

	kd := NewKeyDataMock("explicit-dek")
	KeyDataMockAddVersion("explicit-dek", "mock-key", "v1", util.MustGenerateSecureRandomKey(32))

	dek := util.MustGenerateSecureRandomKey(DataEncryptionKeySize)
	wrapped, err := kd.WrapDataEncryptionKey(ctx, dek)
	require.NoError(t, err)
	require.Equal(t, ProviderTypeMock, wrapped.Provider)
	require.Equal(t, "mock-key", wrapped.ProviderID)
	require.Equal(t, "v1", wrapped.ProviderVersion)
	require.Equal(t, KeyVersionProtectedDataTypeAuthProxyAESGCM, wrapped.ProtectedData.Type)

	unwrapped, err := kd.UnwrapDataEncryptionKey(ctx, DataEncryptionKeyInfo{
		ID:              "dek_explicit",
		EncryptionKeyID: "key_explicit",
		Provider:        wrapped.Provider,
		ProviderID:      wrapped.ProviderID,
		ProviderVersion: wrapped.ProviderVersion,
		ProtectedData:   &wrapped.ProtectedData,
	})
	require.NoError(t, err)
	require.Equal(t, dek, unwrapped)
	require.Equal(t, dek, wrapped.Data)
}

func TestKeyDataNativeGeneratedDataEncryptionKey(t *testing.T) {
	ctx := context.Background()

	ResetKeyDataMockKMSRegistry()
	t.Cleanup(ResetKeyDataMockKMSRegistry)

	kd := NewKeyDataMockKMS("native-generated-dek")
	KeyDataMockKMSAddVersion("native-generated-dek", "mock-kms-key", "v1", util.MustGenerateSecureRandomKey(32))

	_, isNativeGenerator := kd.InnerVal.(KeyDataGeneratesDataEncryptionKeys)
	require.True(t, isNativeGenerator)
	_, isWrapper := kd.InnerVal.(KeyDataWrapsDataEncryptionKeys)
	require.True(t, isWrapper)

	current, err := kd.CurrentWrappingKey(ctx)
	require.NoError(t, err)
	require.Equal(t, ProviderTypeMockKMS, current.Provider)
	require.Equal(t, "mock-kms-key", current.ProviderID)
	require.Equal(t, "v1", current.ProviderVersion)

	generated, err := kd.GenerateDataEncryptionKey(ctx)
	require.NoError(t, err)
	require.Equal(t, ProviderTypeMockKMS, generated.Provider)
	require.Equal(t, "mock-kms-key", generated.ProviderID)
	require.Equal(t, "v1", generated.ProviderVersion)
	require.Len(t, generated.Data, DataEncryptionKeySize)
	require.Equal(t, string(ProviderTypeMockKMS), generated.ProtectedData.Type)
	require.Equal(t, "v1", generated.ProtectedData.Metadata["kek_version"])

	unwrapped, err := kd.UnwrapDataEncryptionKey(ctx, DataEncryptionKeyInfo{
		ID:              "dek_native",
		EncryptionKeyID: "key_native",
		Provider:        generated.Provider,
		ProviderID:      generated.ProviderID,
		ProviderVersion: generated.ProviderVersion,
		ProtectedData:   &generated.ProtectedData,
	})
	require.NoError(t, err)
	require.Equal(t, generated.Data, unwrapped)
}
