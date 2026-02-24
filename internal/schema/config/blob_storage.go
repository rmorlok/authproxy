package config

type BlobStorageProvider string

const (
	BlobStorageProviderS3     BlobStorageProvider = "s3"
	BlobStorageProviderMemory BlobStorageProvider = "memory"
)

// BlobStorageImpl is the interface implemented by concrete blob storage configurations.
type BlobStorageImpl interface {
	GetProvider() BlobStorageProvider
}

// BlobStorage is the holder for a BlobStorageImpl instance.
type BlobStorage struct {
	InnerVal BlobStorageImpl `json:"-" yaml:"-"`
}

func (b *BlobStorage) GetProvider() BlobStorageProvider {
	if b == nil || b.InnerVal == nil {
		return ""
	}
	return b.InnerVal.GetProvider()
}

var _ BlobStorageImpl = (*BlobStorage)(nil)
