//go:build integration

package proxy

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/app_metrics"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// s3BaseURL is the host-facing URL for the integration-tests S3
// service. Mapped to 9003 in integration_tests/docker-compose.yml.
func s3BaseURL() string {
	if endpoint := os.Getenv("AUTHPROXY_TEST_S3_ENDPOINT"); endpoint != "" {
		return endpoint
	}
	return "http://127.0.0.1:9003"
}

// s3StreamingBucket is the anonymous-policy bucket created by the
// SeaweedFS configuration. Public PUT/GET — no AWS Sig v4 required, which
// keeps the test focused on streaming behaviour rather than signing
// machinery.
const s3StreamingBucket = "authproxy-streaming-test"

// uploadSize is large enough that any full-body buffer in the proxy
// path would show up clearly in the runtime.MemStats delta below.
const uploadSize = 50 << 20 // 50 MiB

// TestProxyRaw_S3LargeUpload proves /_proxyRaw streams a large
// upload end-to-end without buffering: the bytes that hit S3 are
// byte-identical (SHA-256 match) to what the client sent, the
// request log marks the body as too-large-skipped (so no tee
// happened), and the process's heap delta during the upload is
// bounded well below the upload size.
//
// Uses a known Content-Length rather than chunked transfer-encoding
// because S3-compatible stores can reject raw chunked PUTs with
// 411 MissingContentLength — the chunked-skip path is already
// covered by TestProxyRawBodyCapture_TeeDecision against an
// httptest.Server. The realistic scenario for AuthProxy users is a
// signed S3 PUT with known length; the property we're asserting
// (no full-body buffer in the proxy) holds for both skip reasons.
//
// The body is a lazy io.LimitReader over crypto/rand teed into a
// sha256.Hash, so neither the client nor the test ever materialises
// the 50 MiB as a contiguous []byte — any large delta in MemStats is
// attributable to AuthProxy itself.
func TestProxyRaw_S3LargeUpload(t *testing.T) {
	connectorID := apid.MustParse("cxr_test0000000000330")

	env := helpers.Setup(t, helpers.SetupOptions{
		Connectors: []sconfig.Connector{
			helpers.NewNoAuthConnector(connectorID, "s3-stream", nil),
		},
		StartHTTPServer: true,
	})
	defer env.Cleanup()
	conn := env.CreateConnection(t, connectorID, 1)

	objectName := fmt.Sprintf("upload-%d.bin", time.Now().UnixNano())
	upstream := fmt.Sprintf("%s/%s/%s", s3BaseURL(), s3StreamingBucket, objectName)
	t.Cleanup(func() { deleteS3Object(t, upstream) })

	// Tee the lazy random stream into the hasher so we know what we
	// sent without ever holding the bytes ourselves.
	hasher := sha256.New()
	body := io.TeeReader(io.LimitReader(rand.Reader, uploadSize), hasher)

	// Sample HeapAlloc *after* a GC so the baseline isn't inflated by
	// outstanding garbage. Skip the post-GC on the way out so we don't
	// blow away allocations the proxy might still be holding — if the
	// proxy buffered the body, we want to see it.
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)

	resp := env.DoProxyRawStreamingRequest(t, conn, upstream, http.MethodPut, body, uploadSize, nil)
	require.NotNil(t, resp)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "s3 response: %s", string(respBody))

	runtime.ReadMemStats(&after)

	expectedHash := hasher.Sum(nil)

	// Re-download from S3 directly (bypassing AuthProxy) and hash
	// what came out.
	gotHash := hashS3Object(t, upstream)
	assert.Equal(t, fmt.Sprintf("%x", expectedHash), fmt.Sprintf("%x", gotHash),
		"S3 object must be byte-identical to what was uploaded")

	// Too-large skip on the request side — proves the roundtripper
	// declined to tee, which is the no-buffer guarantee we care
	// about. 50 MiB ≫ the default max_request_size of 250 KiB.
	record := waitForLog(t, env, conn, 5*time.Second)
	assert.Equal(t, app_metrics.BodySkippedTooLarge, record.RequestBodySkipped,
		"50 MiB upload must record too_large skip reason")

	// Coarse memory check. A 50 MiB full-body buffer in the proxy
	// path would push HeapAlloc up by at least 50 MiB; we allow
	// generous headroom for GC noise, instrumentation maps, and the
	// in-process AuthProxy server itself.
	delta := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	assert.Lessf(t, delta, int64(25<<20),
		"heap grew by %d bytes during a %d-byte upload — looks like the body was buffered",
		delta, uploadSize)
}

// deleteS3Object removes the uploaded object so the bucket doesn't
// accumulate state across runs. Best-effort: failures are logged but
// don't fail the test.
func deleteS3Object(t *testing.T, url string) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, url, nil)
	if err != nil {
		t.Logf("delete %s: build request: %v", url, err)
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Logf("delete %s: %v", url, err)
		return
	}
	resp.Body.Close()
}

// hashS3Object GETs the object directly from S3 and returns the
// SHA-256 of its bytes — streamed, so the test never materialises the
// downloaded body either.
func hashS3Object(t *testing.T, url string) []byte {
	t.Helper()
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "GET %s", url)
	h := sha256.New()
	_, err = io.Copy(h, resp.Body)
	require.NoError(t, err)
	return h.Sum(nil)
}
