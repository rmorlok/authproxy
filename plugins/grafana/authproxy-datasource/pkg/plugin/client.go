package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
)

type authProxyClient struct {
	baseURL    *url.URL
	jwt        string
	httpClient *http.Client
}

func newAuthProxyClient(rawBaseURL, jwt string, httpClient *http.Client) (*authProxyClient, error) {
	if strings.TrimSpace(rawBaseURL) == "" {
		return nil, fmt.Errorf("baseUrl is required")
	}
	if strings.TrimSpace(jwt) == "" {
		return nil, fmt.Errorf("jwt secure setting is required")
	}
	baseURL, err := url.Parse(strings.TrimRight(rawBaseURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("invalid baseUrl: %w", err)
	}
	if baseURL.Scheme == "" || baseURL.Host == "" {
		return nil, fmt.Errorf("baseUrl must include scheme and host")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &authProxyClient{baseURL: baseURL, jwt: jwt, httpClient: httpClient}, nil
}

func (c *authProxyClient) get(ctx context.Context, apiPath string, query url.Values, out any) error {
	u := c.apiURL(apiPath, query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	return c.do(req, out)
}

func (c *authProxyClient) postJSON(ctx context.Context, apiPath string, body any, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	u := c.apiURL(apiPath, nil)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, out)
}

func (c *authProxyClient) do(req *http.Request, out any) error {
	req.Header.Set("Authorization", "Bearer "+c.jwt)
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("authproxy returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode AuthProxy response: %w", err)
	}
	return nil
}

func (c *authProxyClient) apiURL(apiPath string, query url.Values) string {
	u := *c.baseURL
	u.Path = path.Join(u.Path, "/api/v1", apiPath)
	u.RawQuery = query.Encode()
	return u.String()
}

func (c *authProxyClient) health(ctx context.Context) error {
	var out map[string]any
	return c.get(ctx, "/metrics/schema", nil, &out)
}

func (c *authProxyClient) queryMetrics(ctx context.Context, req metricsQueryRequest) (*metricsQueryResponse, error) {
	var out metricsQueryResponse
	if err := c.postJSON(ctx, "/metrics/query", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *authProxyClient) listRequestEvents(ctx context.Context, filters requestEventFilters) ([]requestEvent, error) {
	q := url.Values{}
	addString(q, "namespace", filters.Namespace)
	addString(q, "request_type", filters.RequestType)
	addString(q, "correlation_id", filters.CorrelationID)
	addString(q, "connection_id", filters.ConnectionID)
	addString(q, "connector_type", filters.ConnectorType)
	addString(q, "connector_id", filters.ConnectorID)
	addString(q, "method", filters.Method)
	if filters.StatusCode != 0 {
		q.Set("status_code", strconv.Itoa(filters.StatusCode))
	}
	addString(q, "status_code_range", filters.StatusCodeRange)
	addString(q, "timestamp_range", filters.TimestampRange)
	addString(q, "path", filters.Path)
	addString(q, "path_regex", filters.PathRegex)
	addString(q, "label_selector", filters.LabelSelector)
	addString(q, "response_source", filters.ResponseSource)
	addString(q, "rate_limit_id", filters.RateLimitID)
	q.Set("limit", "500")

	var out listResponse[requestEvent]
	if err := c.get(ctx, "/metrics/request-events", q, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

func (c *authProxyClient) variableValues(ctx context.Context, opts variableQueryOptions) ([]namedResource, error) {
	q := url.Values{}
	q.Set("limit", "500")
	addString(q, "namespace", opts.Namespace)
	addString(q, "label_selector", opts.LabelSelector)

	apiPath := ""
	switch opts.Type {
	case "namespaces":
		apiPath = "/namespaces"
	case "connectors":
		apiPath = "/connectors"
	case "connections":
		apiPath = "/connections"
		addString(q, "connector_id", opts.ConnectorID)
	case "actors":
		apiPath = "/actors"
	case "rate_limits":
		apiPath = "/rate-limits"
	default:
		return nil, fmt.Errorf("unsupported variable query type %q", opts.Type)
	}

	var out listResponse[namedResource]
	if err := c.get(ctx, apiPath, q, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

func addString(q url.Values, key, value string) {
	if strings.TrimSpace(value) != "" {
		q.Set(key, value)
	}
}
