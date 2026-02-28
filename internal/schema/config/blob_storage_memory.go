package config

type BlobStorageMemory struct {
	Provider BlobStorageProvider `json:"provider" yaml:"provider"`
}

func (b *BlobStorageMemory) GetProvider() BlobStorageProvider {
	return BlobStorageProviderMemory
}
