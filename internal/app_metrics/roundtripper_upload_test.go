package app_metrics

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// uploadResult carries completed transport work back to the test goroutine.
type uploadResult struct {
	body []byte
	err  error
}

// earlyRoundTripResult allows the test to require header/error return while the
// simulated transport is still waiting to finish the outgoing request body.
type earlyRoundTripResult struct {
	response *http.Response
	err      error
}

// TestRoundTripper_CaptureAfterEarlyResponse verifies response consumption and
// logging do not assume the transport has finished its request-body goroutine.
func TestRoundTripper_CaptureAfterEarlyResponse(t *testing.T) {
	for _, failure := range []bool{false, true} {
		for _, streaming := range []bool{false, true} {
			name := "response"
			if failure {
				name = "error"
			}
			if streaming {
				name += "/unknown length"
			} else {
				name += "/captured body"
			}
			t.Run(name, func(t *testing.T) {
				// Return the transport result after a prefix, then hold the suffix
				// until the caller has received headers and consumed its response.
				payload := strings.Repeat("upload bytes;", 1024)
				allowUpload := make(chan struct{})
				var releaseOnce sync.Once
				release := func() { releaseOnce.Do(func() { close(allowUpload) }) }
				t.Cleanup(release)
				uploadDone := make(chan uploadResult, 1)
				transportErr := errors.New("early transport error")
				transport := RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
					prefix := make([]byte, 64)
					if _, err := io.ReadFull(req.Body, prefix); err != nil {
						_ = req.Body.Close()
						return nil, err
					}
					go func() {
						<-allowUpload
						tail, readErr := io.ReadAll(req.Body)
						closeErr := req.Body.Close()
						uploadDone <- uploadResult{append(prefix, tail...), errors.Join(readErr, closeErr)}
					}()
					if failure {
						return nil, transportErr
					}
					return &http.Response{
						StatusCode: http.StatusOK, Header: make(http.Header),
						Body: io.NopCloser(strings.NewReader("reply")), ContentLength: 5,
					}, nil
				})
				fullStore := newMockFullStore()
				rt := &RoundTripper{
					store: &mockRecordStore{}, fullStore: fullStore, transport: transport,
					logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
					captureConfig: captureConfig{
						recordFullRequest: true, maxFullRequestSize: uint64(len(payload) + 1),
						maxFullResponseSize: 1024, maxResponseWait: time.Second,
					},
				}
				req, err := http.NewRequest(http.MethodPost, "http://upstream.example/upload", strings.NewReader(payload))
				require.NoError(t, err)
				if streaming {
					req.ContentLength = -1
				}
				returned := make(chan earlyRoundTripResult, 1)
				go func() {
					response, err := rt.RoundTrip(req)
					returned <- earlyRoundTripResult{response, err}
				}()

				// Neither returning headers nor draining a captured response may
				// wait for upload completion, even when both bodies are captured.
				var result earlyRoundTripResult
				select {
				case result = <-returned:
				case <-time.After(time.Second):
					t.Fatal("RoundTrip waited for the unfinished upload")
				}
				if failure {
					require.ErrorIs(t, result.err, transportErr)
				} else {
					require.NoError(t, result.err)
					responseRead := make(chan uploadResult, 1)
					go func() {
						body, readErr := io.ReadAll(result.response.Body)
						closeErr := result.response.Body.Close()
						responseRead <- uploadResult{body, errors.Join(readErr, closeErr)}
					}()
					select {
					case response := <-responseRead:
						require.NoError(t, response.err)
						require.Equal(t, "reply", string(response.body))
					case <-time.After(time.Second):
						t.Fatal("response capture waited for the unfinished upload")
					}
				}
				select {
				case <-fullStore.done:
					t.Fatal("request capture was persisted before upload completion")
				case <-time.After(20 * time.Millisecond):
				}

				// Releasing the suffix permits an exact capture or a final count
				// for streaming bodies, including transport error cleanup.
				release()
				upload := <-uploadDone
				require.NoError(t, upload.err)
				require.Equal(t, payload, string(upload.body))
				fullStore.waitForStore(t, 2*time.Second)
				logs := fullStore.getLogs()
				require.Len(t, logs, 1)
				require.False(t, logs[0].InternalTimeout)
				require.EqualValues(t, len(payload), logs[0].Request.ContentLength)
				if streaming {
					require.Equal(t, BodySkippedStreaming, logs[0].Request.BodySkipped)
					require.Empty(t, logs[0].Request.Body)
				} else {
					require.Equal(t, payload, string(logs[0].Request.Body))
				}
				if !failure {
					require.Equal(t, "reply", string(logs[0].Response.Body))
				}
			})
		}
	}
}
