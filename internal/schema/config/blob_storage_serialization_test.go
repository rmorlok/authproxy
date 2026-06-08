package config

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestBlobStorage_JSONFilesystem(t *testing.T) {
	var bs BlobStorage
	require.NoError(t, json.Unmarshal([]byte(`{
		"provider": "filesystem",
		"path": "/tmp/authproxy/blobs"
	}`), &bs))

	fs, ok := bs.InnerVal.(*BlobStorageFilesystem)
	require.True(t, ok)
	assert.Equal(t, BlobStorageProviderFilesystem, fs.Provider)
	assert.Equal(t, "/tmp/authproxy/blobs", fs.Path)

	data, err := json.Marshal(&bs)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"provider": "filesystem",
		"path": "/tmp/authproxy/blobs"
	}`, string(data))
}

func TestBlobStorage_YAMLFilesystem(t *testing.T) {
	var bs BlobStorage
	require.NoError(t, yaml.Unmarshal([]byte(`
provider: filesystem
path: /tmp/authproxy/blobs
`), &bs))

	fs, ok := bs.InnerVal.(*BlobStorageFilesystem)
	require.True(t, ok)
	assert.Equal(t, BlobStorageProviderFilesystem, fs.Provider)
	assert.Equal(t, "/tmp/authproxy/blobs", fs.Path)

	data, err := yaml.Marshal(&bs)
	require.NoError(t, err)
	assert.Contains(t, string(data), "provider: filesystem")
	assert.Contains(t, string(data), "path: /tmp/authproxy/blobs")
}
