package seeder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	loadtestProviderClientID     = "loadtest-client"
	loadtestProviderClientSecret = "loadtest-secret"
	loadtestProviderClientScope  = "loadtest.read"
)

// HTTPClient is the subset of http.Client used to create the test-mode client.
// It keeps the load-test seeder independently testable.
type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type providerClientRequest struct {
	Key                     string `json:"key"`
	Secret                  string `json:"secret"`
	Scope                   string `json:"scope"`
	TokenEndpointAuthMethod string `json:"token_endpoint_auth_method"`
}

func ensureProviderLoadtestClient(ctx context.Context, client HTTPClient, providerBaseURL string) error {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	payload, err := json.Marshal(providerClientRequest{
		Key:                     loadtestProviderClientID,
		Secret:                  loadtestProviderClientSecret,
		Scope:                   loadtestProviderClientScope,
		TokenEndpointAuthMethod: "client_secret_basic",
	})
	if err != nil {
		return fmt.Errorf("marshal provider load-test client: %w", err)
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		strings.TrimRight(providerBaseURL, "/")+"/test/clients",
		bytes.NewReader(payload),
	)
	if err != nil {
		return fmt.Errorf("create provider load-test client request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("create provider load-test client: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, 8*1024))
	if err != nil {
		return fmt.Errorf("read provider load-test client response: %w", err)
	}
	if response.StatusCode == http.StatusCreated {
		return nil
	}
	if response.StatusCode == http.StatusBadRequest && strings.Contains(strings.ToLower(string(body)), "client id taken") {
		return nil
	}

	return fmt.Errorf("create provider load-test client: unexpected status %s: %s", response.Status, strings.TrimSpace(string(body)))
}
