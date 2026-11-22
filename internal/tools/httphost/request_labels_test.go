package httphost_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	aphttp "github.com/rmorlok/authproxy/internal/apjs/http"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/tools/httphost"
	"github.com/stretchr/testify/require"
)

// requestLabelsClient observes the adapter's chosen request method and detached
// labels without real credentials or network traffic.
type requestLabelsClient struct {
	do func(*http.Request, map[string]string) (*http.Response, error)
}

// Do supplies no override, preserving the configured client's baseline path.
func (c requestLabelsClient) Do(req *http.Request) (*http.Response, error) { return c.do(req, nil) }

// DoWithLabels forwards the private labels selected for this logical request.
func (c requestLabelsClient) DoWithLabels(req *http.Request, labels map[string]string) (*http.Response, error) {
	return c.do(req, labels)
}

// TestSessionRequestLabelsRejectBeforeAdmission leaves allowance and evidence
// unchanged when metadata cannot be accepted, while releasing unsent uploads.
func TestSessionRequestLabelsRejectBeforeAdmission(t *testing.T) {
	var calls int
	client := requestLabelsClient{do: func(req *http.Request, labels map[string]string) (*http.Response, error) {
		calls++
		require.Equal(t, map[string]string{"operation": "read"}, labels)
		require.NoError(t, req.Body.Close())
		httpf.ReportSendState(req.Context(), httpf.SendStateSent)
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: http.NoBody}, nil
	}}
	limits := sessionTestLimits()
	limits.MaxRequests = 1
	session := sessionTestNew(t, context.Background(), client, limits)
	for _, labels := range []map[string]string{
		{"apxy/act/-/id": "forged"}, {"apxy/cxn/-/id": "forged"},
		{"invalid key": "value"}, {"operation": "invalid value"}, {"operation": strings.Repeat("a", 64)},
	} {
		// The no-I/O check and the public Go request path use the same safe error.
		require.ErrorIs(t, session.ValidateLabels(labels), aphttp.ErrInvalidInput)
		request, body := sessionTestRequest(context.Background())
		response, err := session.DoWithLabels(request, labels)
		require.ErrorIs(t, err, aphttp.ErrInvalidInput)
		require.Nil(t, response)
		require.EqualValues(t, 1, body.closes.Load())
		require.Zero(t, body.reads.Load())
	}
	require.Zero(t, calls)
	require.Equal(t, aphttp.SendStateNotSent, session.SendState())

	// Invalid calls consumed no request allowance; the first valid call sends.
	request, body := sessionTestRequest(context.Background())
	response, err := session.DoWithLabels(request, map[string]string{"operation": "read"})
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.EqualValues(t, 1, body.closes.Load())
	require.Equal(t, aphttp.SendStateSent, session.SendState())
	request, body = sessionTestRequest(context.Background())
	response, err = session.DoWithLabels(request, map[string]string{"operation": "write"})
	require.ErrorIs(t, err, aphttp.ErrRequestLimit)
	require.Nil(t, response)
	require.EqualValues(t, 1, body.closes.Load())
	require.Equal(t, 1, calls)
	require.Equal(t, aphttp.SendStateSent, session.SendState())

	// Nil/zero adapters also release transferred bodies without a bound client.
	for _, empty := range []*httphost.Session{nil, {}} {
		require.ErrorIs(t, empty.ValidateLabels(nil), aphttp.ErrInvalidInput)
		request, body := sessionTestRequest(context.Background())
		response, err := empty.DoWithLabels(request, map[string]string{"operation": "read"})
		require.ErrorIs(t, err, aphttp.ErrInvalidInput)
		require.Nil(t, response)
		require.EqualValues(t, 1, body.closes.Load())
	}
}

// TestSessionRequestLabelsConcurrent gives each call a private map while all
// calls retain the same admitted context, deadline, request budget, and evidence.
func TestSessionRequestLabelsConcurrent(t *testing.T) {
	const count = 12
	parent := context.WithValue(context.Background(), integrationContextKey{}, "admitted")
	entered := make(chan struct{}, count)
	release := make(chan struct{})
	var baseline atomic.Int32
	var executionDeadline time.Time
	client := requestLabelsClient{do: func(req *http.Request, labels map[string]string) (*http.Response, error) {
		defer req.Body.Close()
		if labels == nil {
			baseline.Add(1)
		} else {
			// The client may retain or edit its copy without sharing a map with
			// another call or with the Go caller that supplied these labels.
			labels["call"] = req.Header.Get("X-Call")
			entered <- struct{}{}
			<-release
			if labels["call"] != req.Header.Get("X-Call") {
				return nil, fmt.Errorf("request saw another call's label")
			}
		}
		if req.Context().Value(integrationContextKey{}) != "admitted" {
			return nil, fmt.Errorf("request lost admitted context")
		}
		deadline, ok := req.Context().Deadline()
		if !ok || !deadline.Equal(executionDeadline) {
			return nil, fmt.Errorf("request changed execution deadline")
		}
		httpf.ReportSendState(req.Context(), httpf.SendStateSent)
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: http.NoBody}, nil
	}}
	limits := sessionTestLimits()
	limits.MaxRequests = count + 1
	session := sessionTestNew(t, parent, client, limits)
	deadline, ok := session.Context().Deadline()
	require.True(t, ok)
	executionDeadline = deadline
	labels := map[string]string{"operation": "read"}
	results := make(chan error, count)
	var workers sync.WaitGroup
	for index := range count {
		request, _ := sessionTestRequest(context.Background())
		request.Header.Set("X-Call", fmt.Sprint(index))
		workers.Go(func() {
			response, err := session.DoWithLabels(request, labels)
			if response != nil {
				_ = response.Body.Close()
			}
			results <- err
		})
	}
	for range count {
		select {
		case <-entered:
		case err := <-results:
			close(release)
			t.Fatalf("request completed before labeled handoff: %v", err)
		case <-session.Context().Done():
			close(release)
			t.Fatal("requests did not enter the client before their shared deadline")
		}
	}
	close(release)
	workers.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	require.Equal(t, map[string]string{"operation": "read"}, labels)
	require.Equal(t, aphttp.SendStateSent, session.SendState())

	// An unlabeled call uses the same remaining allowance and underlying client.
	request, _ := sessionTestRequest(context.Background())
	response, err := session.Do(request)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.EqualValues(t, 1, baseline.Load())
	request, body := sessionTestRequest(context.Background())
	response, err = session.DoWithLabels(request, map[string]string{"operation": "different"})
	require.ErrorIs(t, err, aphttp.ErrRequestLimit)
	require.Nil(t, response)
	require.EqualValues(t, 1, body.closes.Load())
}

// TestSessionRequestLabelsShareResponseBudget prevents differently labeled
// requests from starting fresh response allowances or erasing earlier evidence.
func TestSessionRequestLabelsShareResponseBudget(t *testing.T) {
	var calls int
	client := requestLabelsClient{do: func(req *http.Request, _ map[string]string) (*http.Response, error) {
		calls++
		require.NoError(t, req.Body.Close())
		httpf.ReportSendState(req.Context(), httpf.SendStateSent)
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("ab"))}, nil
	}}
	limits := sessionTestLimits()
	limits.MaxResponseBytes = 3
	session := sessionTestNew(t, context.Background(), client, limits)
	for index, wantErr := range []error{nil, aphttp.ErrResponseLimit} {
		// The first two-byte body succeeds; a second label cannot reset the
		// shared three-byte cap, and its incomplete body must not be used.
		request, _ := sessionTestRequest(context.Background())
		response, err := session.DoWithLabels(request, map[string]string{"page": fmt.Sprint(index)})
		require.NoError(t, err)
		_, err = io.ReadAll(response.Body)
		require.ErrorIs(t, err, wantErr)
		require.NoError(t, response.Body.Close())
	}

	// Exhaustion cancels the owner, so a newly bound labeled handle cannot send.
	request, body := sessionTestRequest(context.Background())
	response, err := session.DoWithLabels(request, map[string]string{"page": "third"})
	require.ErrorIs(t, err, aphttp.ErrResponseLimit)
	require.Nil(t, response)
	require.EqualValues(t, 1, body.closes.Load())
	require.Equal(t, 2, calls)
	require.Equal(t, aphttp.SendStateSent, session.SendState())
}
