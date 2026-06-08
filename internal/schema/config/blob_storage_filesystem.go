package config

type BlobStorageFilesystem struct {
	Provider BlobStorageProvider `json:"provider" yaml:"provider"`
	Path     string              `json:"path" yaml:"path"`
}

func (b *BlobStorageFilesystem) GetProvider() BlobStorageProvider {
	return BlobStorageProviderFilesystem
}
