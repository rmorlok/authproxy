package apblob

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmorlok/authproxy/internal/schema/config"
)

func newTestFilesystemClient(t *testing.T) (*FilesystemClient, string) {
	t.Helper()
	root := t.TempDir()
	client, err := NewFilesystemClient(&config.BlobStorageFilesystem{
		Provider: config.BlobStorageProviderFilesystem,
		Path:     root,
	})
	require.NoError(t, err)
	fs, ok := client.(*FilesystemClient)
	require.True(t, ok)
	return fs, root
}

func TestFilesystemClient_PutAndGet(t *testing.T) {
	ctx := context.Background()
	c, root := newTestFilesystemClient(t)

	require.NoError(t, c.Put(ctx, PutInput{
		Key:  "root/demo/request.enc",
		Data: []byte("hello world"),
	}))

	data, err := c.Get(ctx, "root/demo/request.enc")
	require.NoError(t, err)
	assert.Equal(t, []byte("hello world"), data)

	onDisk, err := os.ReadFile(filepath.Join(root, "root", "demo", "request.enc"))
	require.NoError(t, err)
	assert.Equal(t, []byte("hello world"), onDisk)
}

func TestFilesystemClient_GetNotFound(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestFilesystemClient(t)

	_, err := c.Get(ctx, "root/missing.enc")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrBlobNotFound))
}

func TestFilesystemClient_PutOverwrite(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestFilesystemClient(t)

	require.NoError(t, c.Put(ctx, PutInput{Key: "root/k.enc", Data: []byte("v1")}))
	require.NoError(t, c.Put(ctx, PutInput{Key: "root/k.enc", Data: []byte("v2")}))

	data, err := c.Get(ctx, "root/k.enc")
	require.NoError(t, err)
	assert.Equal(t, []byte("v2"), data)
}

func TestFilesystemClient_Delete(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestFilesystemClient(t)

	require.NoError(t, c.Put(ctx, PutInput{Key: "root/k.enc", Data: []byte("v")}))
	require.NoError(t, c.Delete(ctx, "root/k.enc"))

	_, err := c.Get(ctx, "root/k.enc")
	assert.True(t, errors.Is(err, ErrBlobNotFound))
}

func TestFilesystemClient_DeleteNonexistent(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestFilesystemClient(t)

	require.NoError(t, c.Delete(ctx, "root/nonexistent.enc"))
}

func TestFilesystemClient_DataIsolation(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestFilesystemClient(t)

	original := []byte("original")
	require.NoError(t, c.Put(ctx, PutInput{Key: "root/k.enc", Data: original}))
	original[0] = 'X'

	data, err := c.Get(ctx, "root/k.enc")
	require.NoError(t, err)
	assert.Equal(t, byte('o'), data[0])

	data[0] = 'Y'
	data2, err := c.Get(ctx, "root/k.enc")
	require.NoError(t, err)
	assert.Equal(t, byte('o'), data2[0])
}

func TestFilesystemClient_RejectsUnsafeKeys(t *testing.T) {
	ctx := context.Background()
	c, root := newTestFilesystemClient(t)

	for _, key := range []string{
		"",
		"/absolute",
		"../escape",
		"root/../escape",
		"root//double",
		"./root",
		"root/.",
		"root/\x00/null",
		`root\windows`,
	} {
		t.Run(key, func(t *testing.T) {
			err := c.Put(ctx, PutInput{Key: key, Data: []byte("x")})
			require.Error(t, err)
		})
	}

	_, err := os.Stat(filepath.Join(filepath.Dir(root), "escape"))
	assert.True(t, errors.Is(err, os.ErrNotExist))
}

func TestNewFilesystemClient_RequiresPath(t *testing.T) {
	client, err := NewFilesystemClient(&config.BlobStorageFilesystem{
		Provider: config.BlobStorageProviderFilesystem,
	})
	require.Error(t, err)
	assert.Nil(t, client)
}
