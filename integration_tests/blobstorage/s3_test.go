//go:build integration

package blobstorage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apblob"
	apconfig "github.com/rmorlok/authproxy/internal/config"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/require"
)

// Exercise the real AWS SDK adapter, including its default checksum handling,
// against the same S3 service used for request recording and proxy streaming.
func TestS3BlobStorage(t *testing.T) {
	cfg, err := apconfig.LoadConfig("../config/integration.yaml")
	require.NoError(t, err)
	storage := cfg.GetRoot().AppMetrics.BlobStorage.InnerVal.(*sconfig.BlobStorageS3)
	if endpoint := os.Getenv("AUTHPROXY_TEST_S3_ENDPOINT"); endpoint != "" {
		storage.Endpoint = endpoint
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := apblob.NewS3Client(ctx, storage)
	require.NoError(t, err)
	key := fmt.Sprintf("compatibility-%d", time.Now().UnixNano())
	data := bytes.Repeat([]byte("authproxy-s3-checksum\n"), 65536)
	require.NoError(t, client.Put(ctx, apblob.PutInput{Key: key, Data: data}))
	t.Cleanup(func() { require.NoError(t, client.Delete(context.Background(), key)) })
	got, err := client.Get(ctx, key)
	require.NoError(t, err)
	require.Equal(t, data, got)
	require.NoError(t, client.Delete(ctx, key))
	_, err = client.Get(ctx, key)
	require.ErrorIs(t, err, apblob.ErrBlobNotFound)

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		t.Run("anonymous_private_bucket_"+method, func(t *testing.T) {
			req, err := http.NewRequestWithContext(ctx, method, storage.Endpoint+"/"+storage.Bucket+"/anonymous-probe", bytes.NewReader([]byte("probe")))
			require.NoError(t, err)
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, http.StatusForbidden, resp.StatusCode, "%s", body)
		})
	}
}
